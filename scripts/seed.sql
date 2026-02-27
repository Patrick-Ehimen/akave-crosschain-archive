-- Seed data for development and testing
-- Run after migrations: psql $DATABASE_URL -f scripts/seed.sql

BEGIN;

-- LayerZero V2: token transfer from Ethereum to Arbitrum (completed)
INSERT INTO messages (message_id, protocol, type, status, created_at, updated_at)
VALUES ('lz-0x00010000000000000001', 'layerzero_v2', 'token_transfer', 'executed', '2025-01-15 10:00:00+00', '2025-01-15 10:02:30+00');

INSERT INTO message_sources (message_id, chain_id, tx_hash, block_number, timestamp, sender, log_index)
VALUES ('lz-0x00010000000000000001', 1, '0xabc123def456789012345678901234567890abcdef1234567890abcdef123456', 19000100, 1705312800, '0x1111111111111111111111111111111111111111', 5);

INSERT INTO message_destinations (message_id, chain_id, tx_hash, block_number, timestamp, receiver, log_index)
VALUES ('lz-0x00010000000000000001', 42161, '0xdef456789012345678901234567890abcdef1234567890abcdef123456789012', 175000200, 1705312950, '0x2222222222222222222222222222222222222222', 3);

INSERT INTO message_payloads (message_id, token, amount, nonce)
VALUES ('lz-0x00010000000000000001', '0xA0b86991c6218b36c1d19D4a2e9Eb0cE3606eB48', '1000000000', 1);

INSERT INTO message_metadata (message_id, fee, relayer, gas_used, latency_seconds)
VALUES ('lz-0x00010000000000000001', '50000000000000', '0x3333333333333333333333333333333333333333', 150000, 150);

-- Wormhole: message from Ethereum to Polygon (pending)
INSERT INTO messages (message_id, protocol, type, status, created_at, updated_at)
VALUES ('wh-1-0xaaa-42', 'wormhole', 'message', 'pending', '2025-01-15 11:00:00+00', '2025-01-15 11:00:00+00');

INSERT INTO message_sources (message_id, chain_id, tx_hash, block_number, timestamp, sender, log_index)
VALUES ('wh-1-0xaaa-42', 1, '0x111222333444555666777888999000aaabbbcccdddeeefffaaa111222333444555', 19000500, 1705316400, '0x4444444444444444444444444444444444444444', 12);

INSERT INTO message_payloads (message_id, data, nonce)
VALUES ('wh-1-0xaaa-42', '0x68656c6c6f', 42);

-- Axelar: contract call from Avalanche to BSC (executed)
INSERT INTO messages (message_id, protocol, type, status, created_at, updated_at)
VALUES ('axl-cmd-0xbbb111', 'axelar', 'contract_call', 'executed', '2025-01-15 12:00:00+00', '2025-01-15 12:05:00+00');

INSERT INTO message_sources (message_id, chain_id, tx_hash, block_number, timestamp, sender, log_index)
VALUES ('axl-cmd-0xbbb111', 43114, '0xaaa111bbb222ccc333ddd444eee555fff666777888999000aaabbbcccdddeee000', 40000300, 1705320000, '0x5555555555555555555555555555555555555555', 8);

INSERT INTO message_destinations (message_id, chain_id, tx_hash, block_number, timestamp, receiver, log_index)
VALUES ('axl-cmd-0xbbb111', 56, '0xbbb222ccc333ddd444eee555fff666777888999000aaabbbcccdddeee000fff111', 35000400, 1705320300, '0x6666666666666666666666666666666666666666', 2);

INSERT INTO message_payloads (message_id, data)
VALUES ('axl-cmd-0xbbb111', '0x095ea7b3000000000000000000000000');

INSERT INTO message_metadata (message_id, fee, gas_used, latency_seconds)
VALUES ('axl-cmd-0xbbb111', '100000000000000', 250000, 300);

-- CCIP: token transfer from Optimism to Base (failed)
INSERT INTO messages (message_id, protocol, type, status, created_at, updated_at)
VALUES ('ccip-msg-0xccc222', 'ccip', 'token_transfer', 'failed', '2025-01-15 13:00:00+00', '2025-01-15 13:10:00+00');

INSERT INTO message_sources (message_id, chain_id, tx_hash, block_number, timestamp, sender, log_index)
VALUES ('ccip-msg-0xccc222', 10, '0xccc333ddd444eee555fff666777888999000aaabbbcccdddeee000fff111aaa222', 110000600, 1705323600, '0x7777777777777777777777777777777777777777', 1);

INSERT INTO message_destinations (message_id, chain_id, tx_hash, block_number, timestamp, receiver, log_index)
VALUES ('ccip-msg-0xccc222', 8453, '0xddd444eee555fff666777888999000aaabbbcccdddeee000fff111aaa222bbb333', 5000700, 1705324200, '0x8888888888888888888888888888888888888888', 4);

INSERT INTO message_payloads (message_id, token, amount, nonce)
VALUES ('ccip-msg-0xccc222', '0x4200000000000000000000000000000000000006', '500000000000000000', 7);

INSERT INTO message_metadata (message_id, fee, relayer, gas_used, latency_seconds)
VALUES ('ccip-msg-0xccc222', '200000000000000', '0x9999999999999999999999999999999999999999', 180000, 600);

-- Indexer cursors: track last indexed block per chain+protocol
INSERT INTO indexer_cursors (chain_id, protocol, last_block, updated_at) VALUES
(1,     'layerzero_v2', 19000100, '2025-01-15 10:00:00+00'),
(1,     'wormhole',     19000500, '2025-01-15 11:00:00+00'),
(42161, 'layerzero_v2', 175000200, '2025-01-15 10:02:30+00'),
(43114, 'axelar',       40000300, '2025-01-15 12:00:00+00'),
(56,    'axelar',       35000400, '2025-01-15 12:05:00+00'),
(10,    'ccip',         110000600, '2025-01-15 13:00:00+00'),
(8453,  'ccip',         5000700, '2025-01-15 13:10:00+00');

COMMIT;
