# Setup Guide

## Prerequisites

| Tool | Version | Purpose |
|------|---------|---------|
| Go | 1.22+ | Build and run the services |
| Docker & Docker Compose | 24+ | Local dev environment |
| PostgreSQL | 15+ | Hot message storage |
| `golang-migrate` | latest | Database migrations |
| `golangci-lint` | latest | Linting (optional, for development) |

Install `golang-migrate`:
```bash
go install -tags 'postgres' github.com/golang-migrate/migrate/v4/cmd/migrate@latest
```

Install `golangci-lint`:
```bash
brew install golangci-lint   # macOS
# or https://golangci-lint.run/usage/install/
```

## 1. Clone the Repository

```bash
git clone https://github.com/Patrick-Ehimen/akave-crosschain-archive.git
cd akave-crosschain-archive
```

## 2. Start the Local Environment

```bash
docker-compose up -d
```

This starts PostgreSQL on port 5432 with the credentials defined in `docker-compose.yml`.

## 3. Configure the Application

Copy the template config and edit it:
```bash
cp configs/config.yaml configs/config.local.yaml
```

Edit `configs/config.local.yaml`:

```yaml
database:
  host: localhost
  port: 5432
  user: crosschain
  password: crosschain
  dbname: crosschain_archive
  sslmode: disable

chains:
  1:
    name: ethereum
    rpc_urls:
      - https://eth.llamarpc.com
      - https://rpc.ankr.com/eth          # fallback
    confirmation_depth: 12
    max_block_range: 1000
    rate_limit: 10
  42161:
    name: arbitrum
    rpc_urls:
      - https://arb1.arbitrum.io/rpc
    confirmation_depth: 1
    max_block_range: 2000
    rate_limit: 20

akave:
  endpoint: o3-rc3.akave.xyz
  access_key: ""    # or set CROSSCHAIN_AKAVE_ACCESS_KEY
  secret_key: ""    # or set CROSSCHAIN_AKAVE_SECRET_KEY
  bucket_name: crosschain-archive
  use_ssl: true
  region: akave-network

indexer:
  batch_size: 1000
  poll_interval: 15s
  archive_interval: 1h

api:
  port: 8080
  read_timeout: 15s
  write_timeout: 30s

logging:
  level: info
  pretty: true   # false → JSON output for production
```

### Environment Variable Overrides

All config keys can be overridden via environment variables with the `CROSSCHAIN_` prefix. Dots become underscores:

| Config key | Environment variable |
|-----------|---------------------|
| `database.host` | `CROSSCHAIN_DATABASE_HOST` |
| `database.password` | `CROSSCHAIN_DATABASE_PASSWORD` |
| `akave.access_key` | `CROSSCHAIN_AKAVE_ACCESS_KEY` |
| `akave.secret_key` | `CROSSCHAIN_AKAVE_SECRET_KEY` |
| `logging.level` | `CROSSCHAIN_LOGGING_LEVEL` |

## 4. Run Database Migrations

```bash
make migrate DB_URL=postgres://crosschain:crosschain@localhost:5432/crosschain_archive?sslmode=disable
```

Or with the default URL:
```bash
make migrate
```

This applies all 5 migrations (schema, correlation indexes, archival tracking, API indexes, block hashes).

## 5. Build Binaries

```bash
make build
```

This produces `bin/indexer` and `bin/api`.

## 6. Run the Services

**Indexer** (indexes blocks and archives to O3):
```bash
CROSSCHAIN_CONFIG=configs/config.local.yaml make run-indexer
# or
CROSSCHAIN_CONFIG=configs/config.local.yaml ./bin/indexer
```

**API server**:
```bash
CROSSCHAIN_CONFIG=configs/config.local.yaml make run-api
# or
CROSSCHAIN_CONFIG=configs/config.local.yaml ./bin/api
```

The API listens on `http://localhost:8080` by default. Prometheus metrics are available at `http://localhost:8080/metrics`.

## 7. Backfill Historical Data

To index a historical block range:
```bash
CROSSCHAIN_CONFIG=configs/config.local.yaml ./bin/backfill \
  --chain=1 \
  --protocol=layerzero \
  --from=19000000 \
  --to=19100000
```

Backfill is idempotent — safe to run multiple times over the same range.

## 8. Run Tests

```bash
# All tests
make test

# With coverage report
make coverage

# Lint
make lint
```

## 9. Docker Production Deployment

Build the image:
```bash
docker build -t crosschain-archive:latest .
```

Run with environment variables:
```bash
docker run -d \
  -e CROSSCHAIN_DATABASE_HOST=your-db-host \
  -e CROSSCHAIN_DATABASE_PASSWORD=your-password \
  -e CROSSCHAIN_AKAVE_ACCESS_KEY=your-access-key \
  -e CROSSCHAIN_AKAVE_SECRET_KEY=your-secret-key \
  -p 8080:8080 \
  crosschain-archive:latest
```

## Configuration Reference

| Key | Default | Description |
|-----|---------|-------------|
| `database.host` | `localhost` | PostgreSQL host |
| `database.port` | `5432` | PostgreSQL port |
| `database.max_open_conns` | `25` | Max open DB connections |
| `chains.<id>.rpc_urls` | — | List of RPC endpoints (first is primary) |
| `chains.<id>.confirmation_depth` | `12` | Blocks to wait before processing |
| `chains.<id>.max_block_range` | `1000` | Max blocks per log query |
| `chains.<id>.rate_limit` | `10` | RPC requests per second |
| `indexer.batch_size` | `1000` | Max blocks processed per cycle |
| `indexer.poll_interval` | `15s` | How often to poll for new blocks |
| `indexer.archive_interval` | `1h` | How often to run the archiver |
| `logging.level` | `info` | Log level: debug, info, warn, error |
| `logging.pretty` | `true` | Human-readable output (false = JSON) |
