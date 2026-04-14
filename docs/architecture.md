# Architecture Overview

CrossChain Archive is a unified indexer and archival system for cross-chain bridge transactions. It ingests events from multiple bridge protocols, normalizes them into a common schema, stores hot data in PostgreSQL, and archives immutable records to Akave O3 in Parquet format.

## High-Level Data Flow

```
EVM Chains (Ethereum, Arbitrum, Optimism, Base, Polygon, Avalanche, BSC)
    │
    ▼
┌─────────────────────────────────────────────────────────────────┐
│  Ingestion Layer (internal/chain)                               │
│  • Multi-endpoint RPC client with automatic failover            │
│  • Token-bucket rate limiting per chain                         │
│  • Block polling with confirmation depth enforcement            │
│  • Reorg detection via parent-hash continuity checks            │
└──────────────────────────┬──────────────────────────────────────┘
                           │ eth.Log[]
                           ▼
┌─────────────────────────────────────────────────────────────────┐
│  Protocol Decoders (internal/decoder)                           │
│  • LayerZero V2 — PacketSent, PacketReceived, OFTSent           │
│  • Wormhole    — LogMessagePublished, TransferRedeemed          │
│  • Axelar      — ContractCall, ContractCallApproved, Executed   │
│  • CCIP        — CCIPSendRequested, ExecutionStateChanged       │
└──────────────────────────┬──────────────────────────────────────┘
                           │ RawEvent
                           ▼
┌─────────────────────────────────────────────────────────────────┐
│  Normalization & Correlation (internal/normalizer, correlator)  │
│  • RawEvent → unified Message schema                            │
│  • Cross-chain message correlation via protocol-specific keys   │
│  • Upsert or correlate against existing pending messages        │
└──────────────────────────┬──────────────────────────────────────┘
                           │ Message
                           ▼
┌─────────────────────────────────────────────────────────────────┐
│  Storage Layer                                                   │
│  ├── Hot: PostgreSQL (internal/storage/postgres)                │
│  │   Messages, sources, destinations, payloads, metadata        │
│  └── Cold: Akave O3 (internal/archiver)                        │
│      Monthly Parquet files, Snappy-compressed, SHA-256 verified │
└─────────────────────────────────────────────────────────────────┘
                           │
                           ▼
┌─────────────────────────────────────────────────────────────────┐
│  Query API (internal/api, cmd/api)                              │
│  • REST endpoints for messages, traces, analytics               │
│  • Historical read-back from O3 (/historical/messages)          │
│  • Prometheus metrics at /metrics                               │
└─────────────────────────────────────────────────────────────────┘
```

## Component Breakdown

### Ingestion Layer (`internal/chain`)

The `Client` wraps go-ethereum's `ethclient` and adds:

- **RPC Failover**: Rotates through configured `rpc_urls` on connection errors. Each failover increments `rpc_failovers_total`.
- **Rate Limiting**: Token-bucket limiter per chain (`rate_limit` requests/second).
- **Retry**: Exponential backoff (1s base, 2x factor, max 3 retries) before failover.
- **Block Fetching**: `FetchLogs(ctx, chainID, fromBlock, toBlock, addresses, topics)` returns matching logs.

### Reorg Detection (`internal/indexer`)

Before processing each block batch, the processor:

1. Records block hashes in the `block_hashes` table (PostgreSQL).
2. Verifies that the live `parentHash` of the first block in the batch matches the stored hash for `blockNumber - 1`.
3. If a mismatch is found (fork), walks back via binary search to find the common ancestor.
4. Rewinds the indexer cursor to the common ancestor block number.
5. Invalidates stale messages from the orphaned blocks.

Pruning keeps only the most recent `reorgPruneDepth` (256) blocks in `block_hashes`.

### Processor (`internal/indexer/processor.go`)

Each chain+protocol pair runs a `Processor` goroutine:

```
PollLoop:
  1. Fetch chain head
  2. Compute safe head (head - confirmationDepth)
  3. Check for reorg since last batch
  4. FetchLogs for cursor..safeHead in batches of maxBlockRange
  5. Decode each log → RawEvent
  6. Normalize + correlate → upsert to PostgreSQL
  7. Advance cursor
  8. Emit indexed_blocks_total, indexed_messages_total, processor_lag_blocks
```

### Archiver (`internal/archiver`)

Runs on a configurable interval (default 1 hour):

1. Queries `messages` for all complete months not yet archived.
2. Serializes matching rows into Parquet with Snappy compression.
3. Computes SHA-256 checksum of the Parquet bytes.
4. Uploads to Akave O3 under key `{protocol}/{chain_id}/{year_month}.parquet`.
5. Records metadata and checksum in `archival_cursors`.
6. Downloads and re-verifies the checksum for integrity confirmation.

### Backfill CLI (`cmd/backfill`)

```bash
backfill --chain=1 --protocol=layerzero --from=19000000 --to=19100000
```

Uses the same processor pipeline but with a `rangedCursorStore` that caps the end block at `--to`. Reorg detection is disabled for backfill since historical data is immutable. Progress is persisted so interrupted backfills can resume.

## Directory Structure

```
crosschain-archive/
├── cmd/
│   ├── indexer/          # Live indexer entrypoint
│   ├── api/              # REST API server entrypoint
│   └── backfill/         # Backfill CLI entrypoint
├── internal/
│   ├── chain/            # RPC client, failover, rate limiting
│   ├── config/           # Viper-based YAML + env config loader
│   ├── decoder/          # Decoder interface, registry, per-protocol implementations
│   │   ├── layerzero/
│   │   ├── wormhole/
│   │   ├── axelar/
│   │   └── ccip/
│   ├── normalizer/       # RawEvent → Message
│   ├── correlator/       # Cross-chain correlation routing
│   ├── indexer/          # Processor, cursor store, block hash store, message store
│   ├── archiver/         # Parquet serialization, O3 upload scheduling
│   ├── api/              # HTTP handlers, router, middleware
│   ├── metrics/          # Prometheus metric definitions
│   ├── logger/           # zerolog factory
│   └── storage/
│       ├── postgres/     # PostgreSQL repository
│       └── akave/        # Akave O3 S3-compatible client
├── migrations/           # golang-migrate SQL files (000001–000005)
├── configs/              # config.yaml template
├── docs/                 # This documentation
├── tests/                # Integration and e2e tests
├── scripts/              # docker-compose, dev tooling
├── Dockerfile
└── Makefile
```

## Observability

| Signal | Tool | Key metrics |
|--------|------|-------------|
| Metrics | Prometheus (`/metrics`) | `indexed_blocks_total`, `indexed_messages_total`, `rpc_request_duration_seconds`, `rpc_failovers_total`, `reorgs_detected_total`, `processor_lag_blocks`, `archived_files_total` |
| Logs | zerolog (JSON in production, pretty in dev) | Structured fields: `chain_id`, `protocol`, `block`, `msg_id`, `error` |

## Persistence Model

```
Hot (PostgreSQL)           Cold (Akave O3)
─────────────────          ────────────────────────────────────────
messages                   {protocol}/{chain_id}/{year_month}.parquet
message_sources              └─ Snappy-compressed Parquet
message_destinations         └─ SHA-256 checksum in archival_cursors
message_payloads
message_metadata
indexer_cursors
archival_cursors
block_hashes
```
