-- CrossChain Archive: initial schema
-- Aligned with internal/types/message.go

BEGIN;

-- messages: core cross-chain message record
CREATE TABLE messages (
    message_id TEXT PRIMARY KEY,
    protocol   TEXT NOT NULL,
    type       TEXT NOT NULL CHECK (type IN ('token_transfer', 'message', 'contract_call')),
    status     TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'executed', 'failed')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- message_sources: originating chain transaction details (1:1 with messages)
CREATE TABLE message_sources (
    message_id   TEXT   PRIMARY KEY REFERENCES messages(message_id) ON DELETE CASCADE,
    chain_id     BIGINT  NOT NULL,
    tx_hash      TEXT    NOT NULL,
    block_number BIGINT  NOT NULL,
    timestamp    BIGINT  NOT NULL,
    sender       TEXT    NOT NULL,
    log_index    INTEGER NOT NULL
);

-- message_destinations: receiving chain transaction details (1:1, added when destination is observed)
CREATE TABLE message_destinations (
    message_id   TEXT   PRIMARY KEY REFERENCES messages(message_id) ON DELETE CASCADE,
    chain_id     BIGINT  NOT NULL,
    tx_hash      TEXT    NOT NULL,
    block_number BIGINT  NOT NULL,
    timestamp    BIGINT  NOT NULL,
    receiver     TEXT    NOT NULL,
    log_index    INTEGER NOT NULL
);

-- message_payloads: transfer/message payload data (1:1)
CREATE TABLE message_payloads (
    message_id TEXT PRIMARY KEY REFERENCES messages(message_id) ON DELETE CASCADE,
    token      TEXT,
    amount     TEXT,
    data       TEXT,
    nonce      BIGINT
);

-- message_metadata: execution details (1:1)
CREATE TABLE message_metadata (
    message_id      TEXT PRIMARY KEY REFERENCES messages(message_id) ON DELETE CASCADE,
    fee             TEXT,
    relayer         TEXT,
    gas_used        BIGINT,
    latency_seconds BIGINT
);

-- indexer_cursors: tracks the last indexed block per chain+protocol pair
CREATE TABLE indexer_cursors (
    chain_id   BIGINT NOT NULL,
    protocol   TEXT   NOT NULL,
    last_block BIGINT NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (chain_id, protocol)
);

-- Indexes on messages
CREATE INDEX idx_messages_protocol   ON messages(protocol);
CREATE INDEX idx_messages_status     ON messages(status);
CREATE INDEX idx_messages_created_at ON messages(created_at);

-- Indexes on message_sources
CREATE INDEX idx_message_sources_chain_id     ON message_sources(chain_id);
CREATE INDEX idx_message_sources_tx_hash      ON message_sources(tx_hash);
CREATE INDEX idx_message_sources_sender       ON message_sources(sender);
CREATE INDEX idx_message_sources_block_number ON message_sources(block_number);
CREATE INDEX idx_message_sources_timestamp    ON message_sources(timestamp);

-- Indexes on message_destinations
CREATE INDEX idx_message_destinations_chain_id  ON message_destinations(chain_id);
CREATE INDEX idx_message_destinations_tx_hash   ON message_destinations(tx_hash);
CREATE INDEX idx_message_destinations_receiver  ON message_destinations(receiver);
CREATE INDEX idx_message_destinations_timestamp ON message_destinations(timestamp);

COMMIT;
