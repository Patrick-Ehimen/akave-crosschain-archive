BEGIN;

DROP TABLE IF EXISTS message_metadata;
DROP TABLE IF EXISTS message_payloads;
DROP TABLE IF EXISTS message_destinations;
DROP TABLE IF EXISTS message_sources;
DROP TABLE IF EXISTS indexer_cursors;
DROP TABLE IF EXISTS messages;

COMMIT;
