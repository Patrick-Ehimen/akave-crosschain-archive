This is the akave-crosschain-archive repo — a unified indexer and archival system for cross-chain bridge transactions.
It indexes from 4 protocols (Wormhole, LayerZero V2, Axelar, Chainlink CCIP), normalizes into a common schema, stores
hot data in PostgreSQL, and archives to Akave O3 in Parquet format. Written in Go.

The project milestones and issues are tracked on a separate repo: https://github.com/akave-ai/akave-pldg (issues
#34-#39). Issue #34 is the tracker. #35-#39 are milestones 1-5.

There is a CLAUDE.md in the repo root with project conventions. There is also a detailed PLAN.md with full
architecture, schemas, and acceptance criteria at https://github.com/akave-ai/akave-pldg on the
crosschain-archive/proposal branch under crosschain-archive/PLAN.md.

## Milestone 1 Progress

Milestone 1 (Project Scaffolding & Core Infrastructure) is largely complete. The following PRs have been merged:

- **PR #9**: Project scaffold — Go module, directory structure, Makefile, docker-compose, Dockerfile, CI, linter config
- **PR #10**: Config loading (Viper), structured logging (zerolog), unified message types, decoder interface + registry
- **PR #11**: Indexer and API entrypoints with config, logging, DB connections, graceful shutdown, chi HTTP server with /health endpoint
- **PR #12**: PostgreSQL schema, migrations (golang-migrate), and storage package (NewPool, RunMigrations)

### Remaining for Milestone 1

- Multi-chain RPC client (`internal/chain/`) with rate limiting and retry
- Akave O3 client wrapper (`internal/storage/akave/`) using MinIO Go client
- Unit tests for RPC and O3 clients
