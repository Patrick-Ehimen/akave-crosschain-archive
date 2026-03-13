-- Add indexes to support efficient PacketReceived correlation lookups.
-- Correlation uses (protocol, nonce, sender, src_chain_id) to match
-- PacketReceived events to existing pending messages.

BEGIN;

CREATE INDEX idx_message_payloads_nonce ON message_payloads(nonce);

CREATE INDEX idx_message_sources_sender_chain ON message_sources(sender, chain_id);

COMMIT;
