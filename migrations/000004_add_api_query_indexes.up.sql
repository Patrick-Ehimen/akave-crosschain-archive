-- Composite indexes for API query patterns (Milestone 4)

BEGIN;

-- GET /messages filtering by protocol + status
CREATE INDEX idx_messages_protocol_status ON messages(protocol, status);

-- GET /messages filtering by protocol + created_at
CREATE INDEX idx_messages_protocol_created_at ON messages(protocol, created_at);

-- GET /messages combined filter for protocol stats
CREATE INDEX idx_messages_protocol_status_created ON messages(protocol, status, created_at);

-- GET /address/{address}/history — sender lookups by timestamp
CREATE INDEX idx_message_sources_sender_timestamp ON message_sources(sender, timestamp);

-- GET /address/{address}/history — receiver lookups by timestamp
CREATE INDEX idx_message_destinations_receiver_timestamp ON message_destinations(receiver, timestamp);

-- GET /routes/stats — source chain with message_id for join
CREATE INDEX idx_source_dest_chain ON message_sources(chain_id) INCLUDE (message_id);

-- GET /messages?token=X — token address lookups on payloads
CREATE INDEX idx_message_payloads_token ON message_payloads(token);

-- Relayer activity lookups
CREATE INDEX idx_message_metadata_relayer ON message_metadata(relayer);

-- Latency range queries for analytics
CREATE INDEX idx_message_metadata_latency ON message_metadata(latency_seconds);

COMMIT;
