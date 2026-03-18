BEGIN;

-- Tracks which (protocol, chain_id, year-month) periods have been archived to O3.
-- Prevents re-archiving unchanged data and serves as the source for manifest generation.
CREATE TABLE archival_cursors (
    protocol      TEXT   NOT NULL,
    chain_id      BIGINT NOT NULL,
    year_month    TEXT   NOT NULL,
    row_count     BIGINT NOT NULL DEFAULT 0,
    min_timestamp BIGINT NOT NULL,
    max_timestamp BIGINT NOT NULL,
    file_size     BIGINT NOT NULL DEFAULT 0,
    object_key    TEXT   NOT NULL,
    archived_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (protocol, chain_id, year_month)
);

COMMIT;
