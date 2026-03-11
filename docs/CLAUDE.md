# CLAUDE.md

## Project Overview

CrossChain Archive — a unified indexer and archival system for cross-chain bridge transactions and messages. Ingests from Wormhole, LayerZero V2, Axelar, and Chainlink CCIP, normalizes into a common schema, stores hot data in PostgreSQL, and archives to Akave O3 in Parquet format.

## Repo

- **Repo**: https://github.com/Patrick-Ehimen/akave-crosschain-archive
- **Issues/Milestones tracked at**: https://github.com/akave-ai/akave-pldg (issues #34-#39)
- **Language**: Go 1.25+

## Architecture

```
EVM Chains → Multi-Chain RPC Client → Protocol Decoders → Normalizer + Correlator
→ PostgreSQL (hot) + Akave O3 (archive, Parquet) → REST API
```

## Key Conventions

- Go project using standard `cmd/` and `internal/` layout
- Two entrypoints: `cmd/indexer/main.go` (indexer service) and `cmd/api/main.go` (API server)
- Protocol decoders implement a shared `Decoder` interface and register via a decoder registry
- Database migrations via `golang-migrate` in `migrations/`
- Config loaded from YAML + environment variable overrides (Viper)
- Structured logging with `zerolog`
- Metrics via Prometheus `client_golang`

## Build & Run

```bash
make build        # Compile
make test         # Run unit tests
make lint         # Lint
make migrate      # Apply DB migrations
make run-indexer  # Start indexer service
make run-api      # Start API server
```

## Dependencies

- `go-ethereum` — ethclient, ABI parsing
- `pgx` + `pgxpool` — PostgreSQL driver
- `golang-migrate` — schema migrations
- `parquet-go` — Parquet serialization
- `chi` — HTTP router
- `viper` — config
- `zerolog` — logging
- `prometheus/client_golang` — metrics
- Akave O3 SDK — object storage

## Database

PostgreSQL 15+ with tables: `messages`, `message_sources`, `message_destinations`, `message_payloads`, `message_metadata`, `indexer_cursors`

## Milestones

1. Project Scaffolding & Core Infrastructure (akave-pldg#35)
2. First Protocol Decoder — LayerZero V2 (akave-pldg#36)
3. Multi-Protocol Expansion (akave-pldg#37)
4. REST API & Query Layer (akave-pldg#38)
5. Production Hardening & Documentation (akave-pldg#39)

## Testing

- Unit tests for decoders with sample ABI-encoded log data
- Integration tests against seeded PostgreSQL
- API endpoint tests with seeded test database
- Target coverage: >70% overall, >85% for decoders

## Style

- Do not add the Co-Authored-By trailer to commits
- Keep commits concise and descriptive
- Follow standard Go conventions (gofmt, golint)
