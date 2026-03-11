package chain

import (
	"context"
	"fmt"
	"math"
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/rs/zerolog"
	"golang.org/x/time/rate"

	"github.com/Patrick-Ehimen/akave-crosschain-archive/internal/config"
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
	Close()
}

// Client wraps a go-ethereum ethclient for a single EVM chain with
// rate limiting and retry logic.
type Client struct {
	eth               EthClient
	chainID           uint64
	name              string
	confirmationDepth uint64
	maxBlockRange     uint64
	limiter           *rate.Limiter
	log               zerolog.Logger
}

// NewClient dials the RPC endpoint for the given chain and returns a ready Client.
func NewClient(ctx context.Context, chainID uint64, cfg config.Chain, log zerolog.Logger) (*Client, error) {
	eth, err := ethclient.DialContext(ctx, cfg.RPCURL)
	if err != nil {
		return nil, fmt.Errorf("dialing RPC for chain %d (%s): %w", chainID, cfg.Name, err)
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
		eth:               eth,
		chainID:           chainID,
		name:              cfg.Name,
		confirmationDepth: cfg.ConfirmationDepth,
		maxBlockRange:     maxBlockRange,
		limiter:           rate.NewLimiter(rate.Limit(rateLimit), rateLimit),
		log:               log.With().Uint64("chain_id", chainID).Str("chain", cfg.Name).Logger(),
	}

	// Verify the connection by fetching the latest block number.
	blockNum, err := c.LatestConfirmedBlock(ctx)
	if err != nil {
		eth.Close()
		return nil, fmt.Errorf("verifying RPC connection for chain %d (%s): %w", chainID, cfg.Name, err)
	}
	c.log.Info().Uint64("confirmed_block", blockNum).Msg("Connected to chain RPC")

	return c, nil
}

// newClientFromEth creates a Client from an existing EthClient (used in tests).
func newClientFromEth(eth EthClient, chainID uint64, name string, confirmationDepth, maxBlockRange uint64, rateLimit int, log zerolog.Logger) *Client {
	if rateLimit <= 0 {
		rateLimit = 10
	}
	if maxBlockRange == 0 {
		maxBlockRange = 1000
	}
	return &Client{
		eth:               eth,
		chainID:           chainID,
		name:              name,
		confirmationDepth: confirmationDepth,
		maxBlockRange:     maxBlockRange,
		limiter:           rate.NewLimiter(rate.Limit(rateLimit), rateLimit),
		log:               log.With().Uint64("chain_id", chainID).Str("chain", name).Logger(),
	}
}

// ChainID returns the chain ID this client is connected to.
func (c *Client) ChainID() uint64 { return c.chainID }

// Name returns the human-readable chain name.
func (c *Client) Name() string { return c.name }

// LatestConfirmedBlock returns the latest block number minus the confirmation depth.
func (c *Client) LatestConfirmedBlock(ctx context.Context) (uint64, error) {
	var blockNum uint64
	err := c.withRetry(ctx, func(ctx context.Context) error {
		if err := c.limiter.Wait(ctx); err != nil {
			return fmt.Errorf("rate limiter: %w", err)
		}
		var err error
		blockNum, err = c.eth.BlockNumber(ctx)
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
		err := c.withRetry(ctx, func(ctx context.Context) error {
			if err := c.limiter.Wait(ctx); err != nil {
				return fmt.Errorf("rate limiter: %w", err)
			}
			var err error
			logs, err = c.eth.FilterLogs(ctx, query)
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

// Close closes the underlying RPC connection.
func (c *Client) Close() {
	c.eth.Close()
}

// withRetry executes fn with exponential backoff retries.
func (c *Client) withRetry(ctx context.Context, fn func(ctx context.Context) error) error {
	var lastErr error
	for attempt := 0; attempt < maxRetries; attempt++ {
		lastErr = fn(ctx)
		if lastErr == nil {
			return nil
		}

		if ctx.Err() != nil {
			return ctx.Err()
		}

		if attempt < maxRetries-1 {
			backoff := time.Duration(float64(baseBackoff) * math.Pow(backoffFactor, float64(attempt)))
			c.log.Warn().
				Err(lastErr).
				Int("attempt", attempt+1).
				Dur("backoff", backoff).
				Msg("RPC call failed, retrying")

			select {
			case <-time.After(backoff):
			case <-ctx.Done():
				return ctx.Err()
			}
		}
	}
	return fmt.Errorf("after %d attempts: %w", maxRetries, lastErr)
}
