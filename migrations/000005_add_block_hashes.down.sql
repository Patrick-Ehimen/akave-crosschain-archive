BEGIN;

ALTER TABLE archival_cursors DROP COLUMN IF EXISTS checksum;
DROP TABLE IF EXISTS block_hashes;

COMMIT;
