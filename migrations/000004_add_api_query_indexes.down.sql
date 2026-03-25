BEGIN;

DROP INDEX IF EXISTS idx_messages_protocol_status;
DROP INDEX IF EXISTS idx_messages_protocol_created_at;
DROP INDEX IF EXISTS idx_messages_protocol_status_created;
DROP INDEX IF EXISTS idx_message_sources_sender_timestamp;
DROP INDEX IF EXISTS idx_message_destinations_receiver_timestamp;
DROP INDEX IF EXISTS idx_source_dest_chain;
DROP INDEX IF EXISTS idx_message_payloads_token;
DROP INDEX IF EXISTS idx_message_metadata_relayer;
DROP INDEX IF EXISTS idx_message_metadata_latency;

COMMIT;
