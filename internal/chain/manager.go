package chain

import (
	"context"
	"fmt"
	"sync"

	"github.com/rs/zerolog"

	"github.com/Patrick-Ehimen/akave-crosschain-archive/internal/config"
)

// Manager holds RPC clients for multiple EVM chains.
type Manager struct {
	mu      sync.RWMutex
	clients map[uint64]*Client
	log     zerolog.Logger
}

// NewManager creates a Client for each configured chain and returns a Manager.
// It connects to every chain and logs the latest confirmed block height.
// If any chain fails to connect, all previously connected clients are closed.
func NewManager(ctx context.Context, chains map[uint64]config.Chain, log zerolog.Logger) (*Manager, error) {
	clients := make(map[uint64]*Client, len(chains))

	for chainID, cfg := range chains {
		client, err := NewClient(ctx, chainID, cfg, log)
		if err != nil {
			// Clean up already-connected clients.
			for _, c := range clients {
				c.Close()
			}
			return nil, fmt.Errorf("connecting to chain %d (%s): %w", chainID, cfg.Name, err)
		}
		clients[chainID] = client
	}

	log.Info().Int("chains", len(clients)).Msg("All chain RPC clients connected")

	return &Manager{
		clients: clients,
		log:     log,
	}, nil
}

// GetClient returns the Client for the given chain ID.
func (m *Manager) GetClient(chainID uint64) (*Client, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	c, ok := m.clients[chainID]
	if !ok {
		return nil, fmt.Errorf("no client for chain %d", chainID)
	}
	return c, nil
}

// AllClients returns a copy of the map of all chain clients.
func (m *Manager) AllClients() map[uint64]*Client {
	m.mu.RLock()
	defer m.mu.RUnlock()

	out := make(map[uint64]*Client, len(m.clients))
	for id, c := range m.clients {
		out[id] = c
	}
	return out
}

// Close closes all underlying RPC connections.
func (m *Manager) Close() {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, c := range m.clients {
		c.Close()
	}
	m.log.Info().Msg("All chain RPC clients closed")
}
