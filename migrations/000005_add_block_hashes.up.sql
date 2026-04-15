BEGIN;

-- Stores the most recent block hashes per chain for reorg detection.
-- The processor validates parent hash continuity before processing each batch.
CREATE TABLE block_hashes (
    chain_id     BIGINT  NOT NULL,
    block_number BIGINT  NOT NULL,
    block_hash   TEXT    NOT NULL,
    parent_hash  TEXT    NOT NULL,
    recorded_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (chain_id, block_number)
);

-- Keep only recent blocks — older ones are pruned after confirmation depth passes.
CREATE INDEX idx_block_hashes_chain_number ON block_hashes (chain_id, block_number DESC);

-- Add checksum column to archival_cursors for integrity verification on read-back.
ALTER TABLE archival_cursors ADD COLUMN checksum TEXT NOT NULL DEFAULT '';

COMMIT;
