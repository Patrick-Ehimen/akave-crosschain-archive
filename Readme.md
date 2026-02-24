# CrossChain Archive

**Unified indexer & archival system for cross-chain bridge transactions and messages.**

CrossChain Archive ingests cross-chain bridge transactions from multiple protocols (Wormhole, LayerZero, Axelar, CCIP), normalizes them into a common schema, stores hot data in PostgreSQL for fast queries, and archives immutable records to [Akave O3](https://console.akave.ai/) in Parquet format.

## Problem

Cross-chain bridges and messaging protocols move billions of dollars across blockchains, but tracking this activity is fragmented:

- **No unified view** across protocols — each has its own explorer
- **Hard to trace end-to-end** — correlating source TX to destination TX requires manual effort
- **Data disappears** — RPC nodes prune old data, making historical analysis difficult
- **Forensics gaps** — when bridges are exploited, investigators lack comprehensive archives
- **No immutable record** — existing indexers run on centralized infrastructure

## Architecture

```
EVM Chains → Ingestion Layer (Multi-Chain RPC) → Protocol Decoders (LZ, Wormhole, Axelar, CCIP)
→ Normalization (Unified Schema + Correlator) → Storage (PostgreSQL hot + Akave O3 archive)
→ Query API (REST endpoints + Trace Engine + Analytics)
```

### Supported Protocols

| Protocol | Source Event | Destination Event | Correlation Key |
|----------|------------|-------------------|-----------------|
| **LayerZero V2** | `PacketSent` | `PacketReceived` | GUID |
| **Wormhole** | `LogMessagePublished` | `TransferRedeemed` | (emitterChain, emitterAddr, sequence) |
| **Axelar** | `ContractCall` | `ContractCallApproved` → `Executed` | commandId |
| **CCIP** | `CCIPSendRequested` | `ExecutionStateChanged` | messageId |

### Supported Chains

Ethereum, Arbitrum, Optimism, Base, Polygon, Avalanche, BSC

## Tech Stack

| Layer | Stack |
|-------|-------|
| **Language** | Go 1.22+ |
| **Ethereum Client** | go-ethereum (ethclient, abi) |
| **Database** | PostgreSQL 15+ |
| **SQL Driver** | pgx + pgxpool |
| **Migrations** | golang-migrate |
| **HTTP Router** | chi or echo |
| **Parquet** | parquet-go |
| **Storage** | Akave O3 |
| **Config** | Viper (YAML + env) |
| **Logging** | zerolog |
| **Metrics** | Prometheus client_golang |
| **CI** | GitHub Actions |
| **Containers** | Docker, Docker Compose |

## Project Layout

```
crosschain-archive/
├── cmd/
│   ├── indexer/              # Indexer service entrypoint
│   └── api/                  # API service entrypoint
├── internal/
│   ├── chain/                # Multi-chain RPC client, block poller
│   ├── decoder/              # Decoder interface + registry
│   │   ├── registry.go
│   │   ├── layerzero/
│   │   ├── wormhole/
│   │   ├── axelar/
│   │   └── ccip/
│   ├── normalizer/           # Raw event → unified message mapping
│   ├── correlator/           # Cross-chain message matching
│   ├── storage/
│   │   ├── postgres/         # PostgreSQL repository
│   │   └── akave/            # Akave O3 client wrapper
│   ├── archiver/             # Parquet serialization + O3 upload scheduling
│   └── api/                  # HTTP handlers, middleware, routes
├── migrations/               # SQL migration files
├── configs/                  # YAML config templates
├── scripts/                  # docker-compose.yml, seed data, dev tooling
├── docs/                     # Architecture, API reference, guides
├── tests/                    # Integration & e2e tests
├── Makefile
├── Dockerfile
└── README.md
```

## API Endpoints

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/messages/{message_id}` | Get message by ID |
| `GET` | `/messages` | Filter messages (src_chain, dst_chain, protocol, status, etc.) |
| `GET` | `/transactions/{tx_hash}/messages` | All messages in a transaction |
| `GET` | `/address/{address}/history` | Cross-chain history for an address |
| `GET` | `/trace/{message_id}` | Full end-to-end trace with all events |
| `GET` | `/protocols/{protocol}/stats` | Volume, latency, success rate |
| `GET` | `/routes/stats` | Popular routes, volume by chain pair |
| `GET` | `/stats/summary` | Aggregate stats across all protocols |
| `GET` | `/health` | Service health check |

## Milestones

- [ ] **Milestone 1**: Project Scaffolding & Core Infrastructure ([#35](https://github.com/akave-ai/akave-pldg/issues/35))
- [ ] **Milestone 2**: First Protocol Decoder — LayerZero V2 ([#36](https://github.com/akave-ai/akave-pldg/issues/36))
- [ ] **Milestone 3**: Multi-Protocol Expansion ([#37](https://github.com/akave-ai/akave-pldg/issues/37))
- [ ] **Milestone 4**: REST API & Query Layer ([#38](https://github.com/akave-ai/akave-pldg/issues/38))
- [ ] **Milestone 5**: Production Hardening & Documentation ([#39](https://github.com/akave-ai/akave-pldg/issues/39))

> Tracker issue: [akave-ai/akave-pldg#34](https://github.com/akave-ai/akave-pldg/issues/34)

## Getting Started

> Coming soon — see Milestone 1 for setup details.

```bash
# Prerequisites
# - Go 1.22+
# - Docker & Docker Compose
# - PostgreSQL 15+

# Clone the repo
git clone https://github.com/Patrick-Ehimen/akave-crosschain-archive.git
cd akave-crosschain-archive

# Start local dev environment
docker-compose up -d

# Run migrations
make migrate

# Start the indexer
make run-indexer

# Start the API server
make run-api
```

## References

- [Akave O3 Console](https://console.akave.ai/)
- [Wormhole Docs](https://docs.wormhole.com/)
- [LayerZero Docs](https://docs.layerzero.network/)
- [Axelar Docs](https://docs.axelar.dev/)
- [Chainlink CCIP Docs](https://docs.chain.link/ccip)

## License

MIT
