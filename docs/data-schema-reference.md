# Data Schema Reference

CrossChain Archive uses a normalized relational schema in PostgreSQL for hot storage and Parquet files in Akave O3 for cold archival.

---

## PostgreSQL Tables

### `messages`

Core cross-chain message record. One row per unique cross-chain message.

| Column | Type | Description |
|--------|------|-------------|
| `message_id` | TEXT (PK) | Unique identifier. Format: `{protocol}-{protocol-specific-key}` |
| `protocol` | TEXT | Protocol name: `layerzero_v2`, `wormhole`, `axelar`, `ccip` |
| `type` | TEXT | Message type: `token_transfer`, `message`, `contract_call` |
| `status` | TEXT | `pending` → `executed` or `failed` |
| `created_at` | TIMESTAMPTZ | When the source event was first indexed |
| `updated_at` | TIMESTAMPTZ | Last status update time |

Indexes: `protocol`, `status`, `created_at`

---

### `message_sources`

Originating chain transaction details. 1:1 with `messages`.

| Column | Type | Description |
|--------|------|-------------|
| `message_id` | TEXT (PK, FK) | References `messages.message_id` |
| `chain_id` | BIGINT | Source chain EIP-155 ID |
| `tx_hash` | TEXT | Source transaction hash |
| `block_number` | BIGINT | Block number of the source event |
| `timestamp` | BIGINT | Unix timestamp of the source block |
| `sender` | TEXT | Address that initiated the cross-chain call |
| `log_index` | INTEGER | Position of the log within the transaction |

Indexes: `chain_id`, `tx_hash`, `sender`, `block_number`, `timestamp`

---

### `message_destinations`

Receiving chain transaction details. 1:1, inserted when the destination event is observed.

| Column | Type | Description |
|--------|------|-------------|
| `message_id` | TEXT (PK, FK) | References `messages.message_id` |
| `chain_id` | BIGINT | Destination chain EIP-155 ID |
| `tx_hash` | TEXT | Destination transaction hash |
| `block_number` | BIGINT | Block number of the destination event |
| `timestamp` | BIGINT | Unix timestamp of the destination block |
| `receiver` | TEXT | Address that received the cross-chain call |
| `log_index` | INTEGER | Position of the log within the transaction |

Indexes: `chain_id`, `tx_hash`, `receiver`, `timestamp`

---

### `message_payloads`

Transfer or message payload data. 1:1 with `messages`.

| Column | Type | Description |
|--------|------|-------------|
| `message_id` | TEXT (PK, FK) | References `messages.message_id` |
| `token` | TEXT | Token contract address (for token transfers) |
| `amount` | TEXT | Transfer amount in base units (wei) as string |
| `data` | TEXT | Arbitrary message data (hex-encoded) |
| `nonce` | BIGINT | Protocol-specific nonce or sequence number |

---

### `message_metadata`

Execution details populated after destination is observed.

| Column | Type | Description |
|--------|------|-------------|
| `message_id` | TEXT (PK, FK) | References `messages.message_id` |
| `fee` | TEXT | Fee paid in wei |
| `relayer` | TEXT | Relayer address that executed the message |
| `gas_used` | BIGINT | Gas consumed on destination |
| `latency_seconds` | BIGINT | Seconds between source and destination timestamps |

---

### `indexer_cursors`

Tracks the last indexed block per chain+protocol pair.

| Column | Type | Description |
|--------|------|-------------|
| `chain_id` | BIGINT (PK) | Chain EIP-155 ID |
| `protocol` | TEXT (PK) | Protocol name |
| `last_block` | BIGINT | Last successfully indexed block |
| `updated_at` | TIMESTAMPTZ | When the cursor was last advanced |

---

### `block_hashes`

Stores recent block hashes for reorg detection. Pruned to the most recent 256 blocks per chain.

| Column | Type | Description |
|--------|------|-------------|
| `chain_id` | BIGINT (PK) | Chain EIP-155 ID |
| `block_number` | BIGINT (PK) | Block number |
| `block_hash` | TEXT | Block hash |
| `parent_hash` | TEXT | Hash of the parent block |
| `recorded_at` | TIMESTAMPTZ | When this record was inserted |

Index: `(chain_id, block_number DESC)` for efficient range queries during reorg detection.

---

### `archival_cursors`

Tracks which (protocol, chain_id, year_month) periods have been archived to Akave O3.

| Column | Type | Description |
|--------|------|-------------|
| `protocol` | TEXT (PK) | Protocol name |
| `chain_id` | BIGINT (PK) | Chain EIP-155 ID |
| `year_month` | TEXT (PK) | Period in `YYYY-MM` format |
| `row_count` | BIGINT | Number of messages in the Parquet file |
| `min_timestamp` | BIGINT | Earliest message timestamp in the file |
| `max_timestamp` | BIGINT | Latest message timestamp in the file |
| `file_size` | BIGINT | Parquet file size in bytes |
| `object_key` | TEXT | O3 object key: `{protocol}/{chain_id}/{year_month}.parquet` |
| `checksum` | TEXT | SHA-256 hex digest of the Parquet file |
| `archived_at` | TIMESTAMPTZ | When the file was uploaded |

---

## Migration History

| Migration | Description |
|-----------|-------------|
| `000001_init_schema` | Core tables: messages, sources, destinations, payloads, metadata, indexer_cursors |
| `000002_add_correlation_index` | Indexes to speed up cross-chain message correlation lookups |
| `000003_add_archival_tracking` | `archival_cursors` table for O3 archival tracking |
| `000004_add_api_query_indexes` | Composite indexes for common API filter patterns |
| `000005_add_block_hashes` | `block_hashes` table for reorg detection; adds `checksum` column to `archival_cursors` |

Run all migrations:
```bash
make migrate
```

Roll back the last migration:
```bash
make migrate-down
```

---

## Parquet Schema (Cold Storage)

Parquet files stored in Akave O3 contain a flat record per message with all fields serialized. The schema mirrors the unified `Message` type from `internal/types`:

| Field | Parquet Type | Description |
|-------|-------------|-------------|
| `message_id` | STRING | Unique message identifier |
| `protocol` | STRING | Protocol name |
| `type` | STRING | `token_transfer`, `message`, or `contract_call` |
| `status` | STRING | Final status at archival time |
| `src_chain_id` | INT64 | Source chain ID |
| `src_tx_hash` | STRING | Source transaction hash |
| `src_block_number` | INT64 | Source block number |
| `src_timestamp` | INT64 | Unix timestamp of source event |
| `src_sender` | STRING | Sender address |
| `dst_chain_id` | INT64 | Destination chain ID (nullable) |
| `dst_tx_hash` | STRING | Destination transaction hash (nullable) |
| `dst_block_number` | INT64 | Destination block number (nullable) |
| `dst_timestamp` | INT64 | Unix timestamp of destination event (nullable) |
| `dst_receiver` | STRING | Receiver address (nullable) |
| `token` | STRING | Token address (nullable) |
| `amount` | STRING | Transfer amount in base units (nullable) |
| `fee` | STRING | Fee paid (nullable) |
| `latency_seconds` | INT64 | End-to-end latency in seconds (nullable) |
| `created_at` | INT64 | Unix timestamp when first indexed |

Compression: **Snappy**. Integrity: SHA-256 checksum stored in `archival_cursors.checksum`.

Object key format: `{protocol}/{chain_id}/{year_month}.parquet`

Example: `layerzero_v2/1/2024-01.parquet`

---

## Message ID Formats

| Protocol | Format | Example |
|----------|--------|---------|
| LayerZero V2 | `lz-{guid}` | `lz-0x1234...abcd` |
| Wormhole | `wh-{emitter_chain}-{emitter_addr}-{sequence}` | `wh-2-0xDEAD...-42` |
| Axelar | `axl-{command_id}` | `axl-0xabcd...` |
| CCIP | `ccip-{message_id}` | `ccip-0x5678...` |
