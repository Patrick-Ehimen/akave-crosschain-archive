BEGIN;

DROP INDEX IF EXISTS idx_message_sources_sender_chain;
DROP INDEX IF EXISTS idx_message_payloads_nonce;

COMMIT;
