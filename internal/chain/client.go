package chain

import (
	"context"
	"fmt"
	"math"
	"math/big"
	"strconv"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/rs/zerolog"
	"golang.org/x/time/rate"

	"github.com/Patrick-Ehimen/akave-crosschain-archive/internal/config"
	"github.com/Patrick-Ehimen/akave-crosschain-archive/internal/metrics"
)

const (
	maxRetries    = 3
	baseBackoff   = 1 * time.Second
	backoffFactor = 2
)

// EthClient defines the subset of ethclient.Client methods used by Client.
// This enables mocking in tests.
type EthClient interface {
	BlockNumber(ctx context.Context) (uint64, error)
	FilterLogs(ctx context.Context, q ethereum.FilterQuery) ([]types.Log, error)
	HeaderByNumber(ctx context.Context, number *big.Int) (*types.Header, error)
	Close()
}

// Client wraps one or more go-ethereum ethclients for a single EVM chain with
// rate limiting, retry logic, and automatic failover across endpoints.
type Client struct {
	mu                sync.Mutex
	endpoints         []EthClient // ordered list; index 0 is current primary
	chainID           uint64
	name              string
	confirmationDepth uint64
	maxBlockRange     uint64
	limiter           *rate.Limiter
	log               zerolog.Logger
	chainIDStr        string // pre-formatted for metric labels
}

// NewClient dials all configured RPC endpoints for the given chain and returns a
// ready Client. If no endpoint is reachable, it returns an error.
func NewClient(ctx context.Context, chainID uint64, cfg config.Chain, log zerolog.Logger) (*Client, error) {
	if len(cfg.RPCURLs) == 0 {
		return nil, fmt.Errorf("no RPC URLs configured for chain %d (%s)", chainID, cfg.Name)
	}

	var endpoints []EthClient
	for _, url := range cfg.RPCURLs {
		eth, err := ethclient.DialContext(ctx, url)
		if err != nil {
			log.Warn().Str("url", url).Err(err).Msg("Failed to dial RPC endpoint, skipping")
			continue
		}
		endpoints = append(endpoints, eth)
	}
	if len(endpoints) == 0 {
		return nil, fmt.Errorf("all RPC endpoints unreachable for chain %d (%s)", chainID, cfg.Name)
	}

	rateLimit := cfg.RateLimit
	if rateLimit <= 0 {
		rateLimit = 10
	}
	maxBlockRange := cfg.MaxBlockRange
	if maxBlockRange == 0 {
		maxBlockRange = 1000
	}

	c := &Client{
		endpoints:         endpoints,
		chainID:           chainID,
		name:              cfg.Name,
		confirmationDepth: cfg.ConfirmationDepth,
		maxBlockRange:     maxBlockRange,
		limiter:           rate.NewLimiter(rate.Limit(rateLimit), rateLimit),
		log:               log.With().Uint64("chain_id", chainID).Str("chain", cfg.Name).Logger(),
		chainIDStr:        strconv.FormatUint(chainID, 10),
	}

	blockNum, err := c.LatestConfirmedBlock(ctx)
	if err != nil {
		c.closeAll()
		return nil, fmt.Errorf("verifying RPC connection for chain %d (%s): %w", chainID, cfg.Name, err)
	}
	c.log.Info().
		Uint64("confirmed_block", blockNum).
		Int("endpoints", len(endpoints)).
		Msg("Connected to chain RPC")

	return c, nil
}

// newClientFromEth creates a Client from existing EthClients (used in tests).
func newClientFromEth(eth EthClient, chainID uint64, name string, confirmationDepth, maxBlockRange uint64, rateLimit int, log zerolog.Logger) *Client {
	if rateLimit <= 0 {
		rateLimit = 10
	}
	if maxBlockRange == 0 {
		maxBlockRange = 1000
	}
	return &Client{
		endpoints:         []EthClient{eth},
		chainID:           chainID,
		name:              name,
		confirmationDepth: confirmationDepth,
		maxBlockRange:     maxBlockRange,
		limiter:           rate.NewLimiter(rate.Limit(rateLimit), rateLimit),
		log:               log.With().Uint64("chain_id", chainID).Str("chain", name).Logger(),
		chainIDStr:        strconv.FormatUint(chainID, 10),
	}
}

// ChainID returns the chain ID this client is connected to.
func (c *Client) ChainID() uint64 { return c.chainID }

// Name returns the human-readable chain name.
func (c *Client) Name() string { return c.name }

// primary returns the currently active EthClient.
func (c *Client) primary() EthClient {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.endpoints[0]
}

// failover rotates the endpoint list, placing the next candidate at index 0.
func (c *Client) failover() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.endpoints) > 1 {
		c.endpoints = append(c.endpoints[1:], c.endpoints[0])
		metrics.RPCFailoversTotal.WithLabelValues(c.chainIDStr).Inc()
		c.log.Warn().Msg("RPC endpoint failover: switched to next endpoint")
	}
}

// LatestConfirmedBlock returns the latest block number minus the confirmation depth.
func (c *Client) LatestConfirmedBlock(ctx context.Context) (uint64, error) {
	var blockNum uint64
	err := c.withRetry(ctx, "BlockNumber", func(ctx context.Context) error {
		if err := c.limiter.Wait(ctx); err != nil {
			return fmt.Errorf("rate limiter: %w", err)
		}
		var err error
		blockNum, err = c.primary().BlockNumber(ctx)
		return err
	})
	if err != nil {
		return 0, fmt.Errorf("fetching block number: %w", err)
	}
	if blockNum <= c.confirmationDepth {
		return 0, nil
	}
	return blockNum - c.confirmationDepth, nil
}

// FetchLogs retrieves logs matching the given filter across a block range.
// The range is automatically chunked according to maxBlockRange.
func (c *Client) FetchLogs(ctx context.Context, fromBlock, toBlock uint64, addresses []common.Address, topics [][]common.Hash) ([]types.Log, error) {
	if fromBlock > toBlock {
		return nil, fmt.Errorf("fromBlock (%d) > toBlock (%d)", fromBlock, toBlock)
	}

	var allLogs []types.Log

	for chunkStart := fromBlock; chunkStart <= toBlock; chunkStart += c.maxBlockRange {
		chunkEnd := chunkStart + c.maxBlockRange - 1
		if chunkEnd > toBlock {
			chunkEnd = toBlock
		}

		query := ethereum.FilterQuery{
			FromBlock: new(big.Int).SetUint64(chunkStart),
			ToBlock:   new(big.Int).SetUint64(chunkEnd),
			Addresses: addresses,
			Topics:    topics,
		}

		var logs []types.Log
		err := c.withRetry(ctx, "FilterLogs", func(ctx context.Context) error {
			if err := c.limiter.Wait(ctx); err != nil {
				return fmt.Errorf("rate limiter: %w", err)
			}
			var err error
			logs, err = c.primary().FilterLogs(ctx, query)
			return err
		})
		if err != nil {
			return nil, fmt.Errorf("fetching logs for blocks %d-%d: %w", chunkStart, chunkEnd, err)
		}

		allLogs = append(allLogs, logs...)

		c.log.Debug().
			Uint64("from", chunkStart).
			Uint64("to", chunkEnd).
			Int("count", len(logs)).
			Msg("Fetched log chunk")
	}

	return allLogs, nil
}

// BlockHeader returns the header for a given block number (used for reorg detection).
func (c *Client) BlockHeader(ctx context.Context, blockNumber uint64) (*types.Header, error) {
	var header *types.Header
	err := c.withRetry(ctx, "HeaderByNumber", func(ctx context.Context) error {
		if err := c.limiter.Wait(ctx); err != nil {
			return fmt.Errorf("rate limiter: %w", err)
		}
		var err error
		header, err = c.primary().HeaderByNumber(ctx, new(big.Int).SetUint64(blockNumber))
		return err
	})
	if err != nil {
		return nil, fmt.Errorf("fetching header for block %d: %w", blockNumber, err)
	}
	return header, nil
}

// BlockTimestamp returns the timestamp (unix seconds) for a given block number.
func (c *Client) BlockTimestamp(ctx context.Context, blockNumber uint64) (int64, error) {
	header, err := c.BlockHeader(ctx, blockNumber)
	if err != nil {
		return 0, err
	}
	return int64(header.Time), nil
}

// Close closes all underlying RPC connections.
func (c *Client) Close() {
	c.closeAll()
}

func (c *Client) closeAll() {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, e := range c.endpoints {
		e.Close()
	}
}

// withRetry executes fn with exponential backoff retries. On each failure it
// triggers a failover so the next attempt uses a different RPC endpoint.
func (c *Client) withRetry(ctx context.Context, method string, fn func(ctx context.Context) error) error {
	var lastErr error
	for attempt := 0; attempt < maxRetries; attempt++ {
		start := time.Now()
		lastErr = fn(ctx)
		duration := time.Since(start).Seconds()

		metrics.RPCRequestDurationSeconds.WithLabelValues(c.chainIDStr, method).Observe(duration)

		if lastErr == nil {
			return nil
		}

		if ctx.Err() != nil {
			return ctx.Err()
		}

		backoff := time.Duration(float64(baseBackoff) * math.Pow(backoffFactor, float64(attempt)))
		c.log.Warn().
			Err(lastErr).
			Str("method", method).
			Int("attempt", attempt+1).
			Dur("backoff", backoff).
			Msg("RPC call failed, failing over and retrying")

		c.failover()

		if attempt < maxRetries-1 {
			select {
			case <-time.After(backoff):
			case <-ctx.Done():
				return ctx.Err()
			}
		}
	}
	return fmt.Errorf("after %d attempts: %w", maxRetries, lastErr)
}
