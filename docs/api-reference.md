# API Reference

Base URL: `http://localhost:8080` (configurable via `api.port`)

All responses are JSON. Error responses follow the format:
```json
{"error": "description"}
```

---

## Health

### `GET /health`

Returns service health status including database connectivity and indexer cursor state.

**Response 200:**
```json
{
  "status": "ok",
  "database": "connected",
  "indexer": {
    "cursors": [
      {
        "chain_id": 1,
        "protocol": "layerzero_v2",
        "last_block": 19500000,
        "updated_at": "2024-01-15T12:00:00Z"
      }
    ]
  }
}
```

**Response 503** (database unreachable):
```json
{
  "status": "degraded",
  "database": "disconnected",
  "error": "connection refused"
}
```

---

## Messages

### `GET /messages`

List cross-chain messages with optional filters.

**Query Parameters:**

| Parameter | Type | Description |
|-----------|------|-------------|
| `protocol` | string | Filter by protocol (`layerzero_v2`, `wormhole`, `axelar`, `ccip`) |
| `src_chain` | uint64 | Source chain ID |
| `dst_chain` | uint64 | Destination chain ID |
| `status` | string | `pending`, `executed`, `failed` |
| `sender` | string | Source address (0x-prefixed) |
| `receiver` | string | Destination address (0x-prefixed) |
| `from_time` | int64 | Unix timestamp — messages after this time |
| `to_time` | int64 | Unix timestamp — messages before this time |
| `limit` | int | Max results (default 50, max 1000) |
| `offset` | int | Pagination offset |

**Response 200:**
```json
{
  "messages": [
    {
      "message_id": "lz-0x1234...-42",
      "protocol": "layerzero_v2",
      "type": "token_transfer",
      "status": "executed",
      "created_at": "2024-01-15T10:00:00Z",
      "updated_at": "2024-01-15T10:05:00Z",
      "source": {
        "chain_id": 1,
        "tx_hash": "0xabc...",
        "block_number": 19400000,
        "timestamp": 1705312800,
        "sender": "0xDEAD...",
        "log_index": 3
      },
      "destination": {
        "chain_id": 42161,
        "tx_hash": "0xdef...",
        "block_number": 180000000,
        "timestamp": 1705313100,
        "receiver": "0xBEEF...",
        "log_index": 1
      },
      "payload": {
        "token": "0xA0b8...",
        "amount": "1000000000000000000",
        "data": "",
        "nonce": 12345
      },
      "metadata": {
        "fee": "100000000000000",
        "relayer": "0x...",
        "gas_used": 150000,
        "latency_seconds": 300
      }
    }
  ],
  "total": 1
}
```

---

### `GET /messages/{message_id}`

Retrieve a single message by ID.

**Response 200:** Same message object as above.

**Response 404:**
```json
{"error": "message not found"}
```

---

### `GET /transactions/{tx_hash}/messages`

All cross-chain messages originating from a transaction hash.

**Response 200:**
```json
{
  "tx_hash": "0xabc...",
  "messages": [ /* array of message objects */ ]
}
```

---

## Address History

### `GET /address/{address}/history`

Cross-chain transaction history for an Ethereum address (as sender or receiver).

**Query Parameters:**

| Parameter | Type | Description |
|-----------|------|-------------|
| `protocol` | string | Filter by protocol |
| `limit` | int | Max results (default 50) |
| `offset` | int | Pagination offset |

**Response 200:**
```json
{
  "address": "0xDEAD...",
  "messages": [ /* array of message objects */ ]
}
```

---

## Trace

### `GET /trace/{message_id}`

Full end-to-end trace for a cross-chain message, including all observed events.

**Response 200:**
```json
{
  "message_id": "lz-0x1234...-42",
  "protocol": "layerzero_v2",
  "status": "executed",
  "events": [
    {
      "event_type": "PacketSent",
      "chain_id": 1,
      "tx_hash": "0xabc...",
      "block_number": 19400000,
      "timestamp": 1705312800,
      "data": {
        "guid": "0x1234...",
        "dst_eid": "30110"
      }
    },
    {
      "event_type": "PacketReceived",
      "chain_id": 42161,
      "tx_hash": "0xdef...",
      "block_number": 180000000,
      "timestamp": 1705313100,
      "data": {
        "guid": "0x1234..."
      }
    }
  ],
  "latency_seconds": 300
}
```

---

## Analytics

### `GET /protocols/{protocol}/stats`

Volume, latency, and success rate for a protocol.

**Path Parameters:** `protocol` — one of `layerzero_v2`, `wormhole`, `axelar`, `ccip`

**Response 200:**
```json
{
  "protocol": "layerzero_v2",
  "total_messages": 15420,
  "executed": 15100,
  "failed": 120,
  "pending": 200,
  "avg_latency_seconds": 287,
  "p50_latency_seconds": 250,
  "p95_latency_seconds": 600,
  "volume_usd": "45000000"
}
```

---

### `GET /routes/stats`

Popular routes ranked by volume, broken down by chain pair.

**Response 200:**
```json
{
  "routes": [
    {
      "src_chain_id": 1,
      "dst_chain_id": 42161,
      "protocol": "layerzero_v2",
      "message_count": 5200,
      "volume_usd": "12000000"
    }
  ]
}
```

---

### `GET /stats/summary`

Aggregate stats across all protocols.

**Response 200:**
```json
{
  "total_messages": 48000,
  "executed": 46500,
  "failed": 500,
  "pending": 1000,
  "protocols": ["layerzero_v2", "wormhole", "axelar", "ccip"],
  "chains": [1, 42161, 10, 8453]
}
```

---

## Historical (Archived Data)

These endpoints serve data from Akave O3 for periods that have been rotated out of hot PostgreSQL storage.

### `GET /historical/index`

List all available archived periods.

**Response 200:**
```json
{
  "total": 3,
  "files": [
    {
      "protocol": "layerzero_v2",
      "chain_id": 1,
      "year_month": "2024-01",
      "row_count": 12500,
      "file_size": 4194304,
      "object_key": "layerzero_v2/1/2024-01.parquet",
      "checksum": "sha256:abc123...",
      "min_timestamp": "2024-01-01T00:00:00Z",
      "max_timestamp": "2024-01-31T23:59:59Z",
      "recorded_at": "2024-02-01T01:00:00Z"
    }
  ]
}
```

---

### `GET /historical/messages`

Download and deserialize archived messages for a specific period.

**Query Parameters (all required):**

| Parameter | Type | Description |
|-----------|------|-------------|
| `protocol` | string | Protocol name |
| `chain_id` | uint64 | Chain ID |
| `year_month` | string | Period in `YYYY-MM` format |

**Response Headers:**

| Header | Description |
|--------|-------------|
| `X-Archive-Object` | O3 object key of the Parquet file |
| `X-Archive-Checksum` | SHA-256 checksum |
| `X-Archive-Row-Count` | Number of records in the file |

**Response 200:**
```json
{
  "object_key": "layerzero_v2/1/2024-01.parquet",
  "year_month": "2024-01",
  "protocol": "layerzero_v2",
  "chain_id": 1,
  "checksum": "abc123...",
  "row_count": 12500,
  "records": [ /* array of serialized message objects */ ]
}
```

**Response 400:** Missing required query parameters.

**Response 404:** No archived data found for the requested period.

**Response 502:** Failed to download file from O3.

---

## Observability

### `GET /metrics`

Prometheus metrics endpoint. Returns all registered metrics in text exposition format.

**Key metrics:**

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `indexed_blocks_total` | Counter | `chain_id`, `protocol` | Blocks successfully indexed |
| `indexed_messages_total` | Counter | `chain_id`, `protocol`, `event_type` | Messages indexed |
| `rpc_request_duration_seconds` | Histogram | `chain_id`, `method` | RPC call latency |
| `rpc_failovers_total` | Counter | `chain_id` | RPC endpoint switchovers |
| `reorgs_detected_total` | Counter | `chain_id`, `protocol` | Chain reorgs detected |
| `processor_lag_blocks` | Gauge | `chain_id`, `protocol` | Blocks behind chain head |
| `archived_files_total` | Counter | `protocol`, `chain_id` | Parquet files archived to O3 |
