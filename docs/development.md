# Development Context & Milestone 1 Plan

## Current State

- Milestone 1 (akave-pldg#35) is in progress — most phases complete
- Implemented: Go module, project structure, Makefile, docker-compose, Dockerfile, CI, config loading, logging, unified types, decoder interface/registry, PostgreSQL schema + migrations, storage package, indexer + API entrypoints
- Remaining: multi-chain RPC client (`internal/chain/`), Akave O3 client wrapper (`internal/storage/akave/`)

## References

- **Tracker**: https://github.com/akave-ai/akave-pldg/issues/34
- **Milestone 1 issue**: https://github.com/akave-ai/akave-pldg/issues/35
- **Full PLAN.md**: https://github.com/akave-ai/akave-pldg/pull/5 (branch `crosschain-archive/proposal`, file `crosschain-archive/PLAN.md`)

## Milestone 1 Tasks

1. Initialize Go module (`github.com/Patrick-Ehimen/akave-crosschain-archive`) and project directory structure
2. Set up `Makefile` with targets: `build`, `test`, `lint`, `migrate`, `run-indexer`, `run-api`
3. Add `docker-compose.yml` for local PostgreSQL
4. Configure CI with GitHub Actions (lint + unit tests)
5. Implement config loading from YAML + environment variable overrides (Viper)
6. Design and apply PostgreSQL schema via `golang-migrate` migrations
7. Implement multi-chain RPC client with configurable confirmation depth and rate limiting
8. Implement Akave O3 client wrapper (`Upload`, `Download`, `List`) with retry logic
9. Write seed SQL and unit tests for RPC and O3 clients

## Key Decisions Made

| Decision | Choice | Rationale |
|----------|--------|-----------|
| Akave O3 client | MinIO Go client (`minio-go/v7`) | O3 exposes S3-compatible API; simplest approach |
| HTTP router | `go-chi/chi/v5` | Lightweight, stdlib-compatible |
| PostgreSQL driver | `jackc/pgx/v5` + pgxpool | Best Go PG driver, connection pooling |
| Config | `spf13/viper` | YAML + env var overrides |
| Logging | `rs/zerolog` | Structured, fast, zero-allocation |
| Migrations | `golang-migrate/migrate/v4` | Standard Go migration tool |

## Implementation Plan

### Phase 1: Go Module & Project Structure

Initialize Go module and create directory tree:

```
cmd/indexer/              # Indexer service entrypoint (main.go)
cmd/api/                  # API service entrypoint (main.go)
internal/config/          # Config loading (Viper)
internal/logger/          # Zerolog setup
internal/types/           # Unified message types
internal/chain/           # Multi-chain RPC client + manager
internal/decoder/         # Decoder interface + registry (stubs)
internal/decoder/layerzero/
internal/decoder/wormhole/
internal/decoder/axelar/
internal/decoder/ccip/
internal/normalizer/      # Raw event -> unified message (M2+)
internal/correlator/      # Cross-chain matching (M2+)
internal/storage/postgres/ # PostgreSQL repository + migrations
internal/storage/akave/   # Akave O3 client wrapper (MinIO)
internal/archiver/        # Parquet + O3 upload scheduling (M2+)
internal/api/             # HTTP handlers, middleware (M4)
migrations/               # SQL migration files
configs/                  # YAML config templates
scripts/                  # docker-compose, seed data
docs/                     # This file + future docs
tests/                    # Integration & e2e tests
```

### Phase 2: Build System & Docker

- **Makefile**: `build`, `test`, `lint`, `migrate`, `run-indexer`, `run-api`
- **`scripts/docker-compose.yml`**: PostgreSQL 15, volume, health check, port 5432
- **Dockerfile**: Multi-stage (Go builder -> minimal runtime)

### Phase 3: Configuration

- **`internal/config/config.go`**: Config struct with Database, Chains (map), Akave, Indexer sections. `Load(path)` via Viper. `CROSSCHAIN_` env var prefix.
- **`configs/config.yaml`**: Template with defaults and comments
- **`internal/config/config_test.go`**: Unit tests

### Phase 4: Database Schema & Migrations

**`migrations/000001_init_schema.up.sql`**:
- `messages`: message_id (PK), protocol, type, status, created_at, updated_at
- `message_sources`: message_id (FK), chain_id, tx_hash, block_number, timestamp, sender, log_index
- `message_destinations`: message_id (FK), chain_id, tx_hash, block_number, timestamp, receiver, log_index
- `message_payloads`: message_id (FK), token, amount, data, nonce
- `message_metadata`: message_id (FK), fee, relayer, gas_used, latency_seconds
- `indexer_cursors`: chain_id + protocol (composite PK), last_block, updated_at
- Indexes on: protocol, status, chain_id, tx_hash, sender/receiver, timestamps

**`internal/storage/postgres/postgres.go`**: `NewPool()`, `RunMigrations()`

### Phase 5: Multi-Chain RPC Client

- **`internal/chain/client.go`**: Wraps go-ethereum ethclient per chain. `LatestConfirmedBlock()`, `FetchLogs()`. Rate limiting (`golang.org/x/time/rate`), retry with exponential backoff.
- **`internal/chain/manager.go`**: Manages multiple chain clients. `GetClient(chainID)`, `AllClients()`.
- **`internal/chain/client_test.go`**: Tests for rate limiting, block range chunking, retry behavior.

### Phase 6: Akave O3 Client Wrapper

- **`internal/storage/akave/client.go`**: Wraps `minio.Client`. `Upload()`, `Download()`, `List()`, `Delete()`. Retry with exponential backoff (3 attempts).
- **`internal/storage/akave/client_test.go`**: Tests for retry logic, error handling.

### Phase 7: Decoder Interface (Stubs for M2+)

- **`internal/decoder/types.go`**: `RawEvent` struct, `Decoder` interface
- **`internal/decoder/registry.go`**: `Registry` with Register/Get/All

### Phase 8: Logging & Shared Types

- **`internal/logger/logger.go`**: `NewLogger(level, pretty)` — console for dev, JSON for prod
- **`internal/types/message.go`**: Unified Message, Source, Destination, Payload, Metadata structs

### Phase 9: CI

- **`.github/workflows/ci.yml`**: lint (golangci-lint), test, build. Go 1.25, PG service container.
- **`.golangci.yml`**: Linter config

## Files (26 total)

| # | File | Purpose | Status |
|---|------|---------|--------|
| 1 | `go.mod` | Go module init | Done |
| 2 | `Makefile` | Build targets | Done |
| 3 | `scripts/docker-compose.yml` | Local PostgreSQL | Done |
| 4 | `Dockerfile` | Container build | Done |
| 5 | `configs/config.yaml` | Config template | Done |
| 6 | `internal/config/config.go` | Config loading (Viper) | Done |
| 7 | `internal/config/config_test.go` | Config tests | Done |
| 8 | `internal/logger/logger.go` | Zerolog setup | Done |
| 9 | `internal/types/message.go` | Unified message types | Done |
| 10 | `internal/decoder/types.go` | Decoder interface + RawEvent | Done |
| 11 | `internal/decoder/registry.go` | Decoder registry | Done |
| 12 | `migrations/000001_init_schema.up.sql` | DB schema | Done |
| 13 | `migrations/000001_init_schema.down.sql` | Schema rollback | Done |
| 14 | `internal/storage/postgres/postgres.go` | DB pool + migrations | Done |
| 15 | `internal/storage/postgres/postgres_test.go` | DB tests | Done |
| 16 | `internal/chain/client.go` | RPC client per chain | TODO |
| 17 | `internal/chain/manager.go` | Multi-chain manager | TODO |
| 18 | `internal/chain/client_test.go` | RPC client tests | TODO |
| 19 | `internal/storage/akave/client.go` | O3 S3-compatible wrapper | TODO |
| 20 | `internal/storage/akave/client_test.go` | O3 client tests | TODO |
| 21 | `cmd/indexer/main.go` | Indexer entrypoint | Done |
| 22 | `cmd/api/main.go` | API entrypoint | Done |
| 23 | `scripts/seed.sql` | Seed data | Done |
| 24 | `.github/workflows/ci.yml` | CI pipeline | Done |
| 25 | `.golangci.yml` | Linter config | Done |
| 26 | `docs/development.md` | This file | Done |

## Go Dependencies

```
github.com/ethereum/go-ethereum     # ethclient, ABI, types
github.com/jackc/pgx/v5             # PostgreSQL driver + pool
github.com/golang-migrate/migrate/v4 # DB migrations
github.com/minio/minio-go/v7        # S3-compatible client for Akave O3
github.com/spf13/viper              # Config loading
github.com/rs/zerolog               # Structured logging
github.com/go-chi/chi/v5            # HTTP router (stub in M1)
golang.org/x/time                   # Rate limiter
github.com/stretchr/testify         # Test assertions
```

## Acceptance Criteria

- `make build` compiles both binaries without errors
- `make test` passes all unit tests
- `docker compose -f scripts/docker-compose.yml up -d` starts PostgreSQL
- `make migrate` applies schema to a fresh PostgreSQL instance
- Indexer binary starts, connects to RPCs, logs current block heights
- Config loads from YAML with env var overrides working
- A test file can be uploaded to and retrieved from Akave O3

## Akave O3 Notes

- O3 is S3-compatible, so we use MinIO Go client
- Auth: Access Key + Secret Key (like AWS S3)
- Endpoint: configurable (e.g., `o3-rc2.akave.xyz`)
- Region: `akave-network`
- Supports: multipart upload, presigned URLs, bucket policies
- Credentials obtained from Akave directly
