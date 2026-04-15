package indexer

import (
	"context"
	"testing"
	"time"

	"github.com/Patrick-Ehimen/akave-crosschain-archive/internal/normalizer"
	"github.com/Patrick-Ehimen/akave-crosschain-archive/internal/types"
)

// ─── Cross-protocol DB integration tests ──────────────────────────────────
//
// These tests verify that all 4 protocols' messages are correctly stored,
// queried, and correlated in PostgreSQL using the real PostgresMessageStore.
// They auto-skip if PostgreSQL is unavailable via testhelper.NewTestPool.

// protocolTestCase defines a message fixture for one protocol.
type protocolTestCase struct {
	name     string
	msgID    string
	protocol string
	msgType  types.MessageType
	chainID  uint64
	sender   string
	nonce    uint64
	token    string
	amount   string
	fee      string
}

var crossProtocolCases = []protocolTestCase{
	{
		name:     "LayerZero",
		msgID:    "xproto-lz-001",
		protocol: "layerzero_v2",
		msgType:  types.TypeMessage,
		chainID:  1,
		sender:   "0xSenderLZ1111111111111111111111111111111111",
		nonce:    42,
	},
	{
		name:     "Wormhole",
		msgID:    "xproto-wh-001",
		protocol: "wormhole",
		msgType:  types.TypeMessage,
		chainID:  1,
		sender:   "0x3ee18B2214AFF97000D974cf647E7C347E8fa585",
		nonce:    42,
	},
	{
		name:     "Axelar",
		msgID:    "xproto-axl-001",
		protocol: "axelar",
		msgType:  types.TypeContractCall,
		chainID:  1,
		sender:   "0xSenderAXL1111111111111111111111111111111111",
		nonce:    0, // Axelar uses MessageID, not nonce
	},
	{
		name:     "CCIP",
		msgID:    "xproto-ccip-001",
		protocol: "ccip",
		msgType:  types.TypeTokenTransfer,
		chainID:  1,
		sender:   "0xSenderCCIP111111111111111111111111111111111",
		nonce:    7,
		token:    "0xA0b86991c6218b36c1d19D4a2e9Eb0cE3606eB48",
		amount:   "1000000000",
		fee:      "100000000000000",
	},
}

func newCrossProtocolMessage(tc protocolTestCase) *types.Message {
	now := time.Now()
	msg := &types.Message{
		MessageID: tc.msgID,
		Protocol:  tc.protocol,
		Type:      tc.msgType,
		Status:    types.StatusPending,
		Source: types.Source{
			ChainID:     tc.chainID,
			TxHash:      "0x" + tc.protocol + "_src_tx",
			BlockNumber: 19000000,
			Timestamp:   1706000000,
			Sender:      tc.sender,
			LogIndex:    5,
		},
		Payload: &types.Payload{
			Data:  "0xdeadbeef",
			Nonce: tc.nonce,
		},
		Metadata:  &types.Metadata{},
		CreatedAt: now,
		UpdatedAt: now,
	}
	if tc.token != "" {
		msg.Payload.Token = tc.token
		msg.Payload.Amount = tc.amount
	}
	if tc.fee != "" {
		msg.Metadata.Fee = tc.fee
	}
	return msg
}

func TestCrossProtocol_UpsertAndQuery_AllProtocols(t *testing.T) {
	store := newTestMessageStore(t)
	ctx := context.Background()

	for _, tc := range crossProtocolCases {
		t.Run(tc.name, func(t *testing.T) {
			msg := newCrossProtocolMessage(tc)
			t.Cleanup(func() { cleanupMessage(t, store, tc.msgID) })

			if err := store.UpsertMessage(ctx, msg); err != nil {
				t.Fatalf("UpsertMessage failed: %v", err)
			}

			// Verify messages table
			var protocol, msgType, status string
			err := store.pool.QueryRow(ctx,
				"SELECT protocol, type, status FROM messages WHERE message_id = $1", tc.msgID,
			).Scan(&protocol, &msgType, &status)
			if err != nil {
				t.Fatalf("query messages: %v", err)
			}
			if protocol != tc.protocol {
				t.Errorf("protocol = %q, want %q", protocol, tc.protocol)
			}
			if msgType != string(tc.msgType) {
				t.Errorf("type = %q, want %q", msgType, tc.msgType)
			}
			if status != "pending" {
				t.Errorf("status = %q, want pending", status)
			}

			// Verify message_sources table
			var chainID int64
			var sender string
			err = store.pool.QueryRow(ctx,
				"SELECT chain_id, sender FROM message_sources WHERE message_id = $1", tc.msgID,
			).Scan(&chainID, &sender)
			if err != nil {
				t.Fatalf("query sources: %v", err)
			}
			if uint64(chainID) != tc.chainID {
				t.Errorf("chain_id = %d, want %d", chainID, tc.chainID)
			}
			if sender != tc.sender {
				t.Errorf("sender = %q, want %q", sender, tc.sender)
			}

			// Verify message_payloads table
			var nonce int64
			err = store.pool.QueryRow(ctx,
				"SELECT nonce FROM message_payloads WHERE message_id = $1", tc.msgID,
			).Scan(&nonce)
			if err != nil {
				t.Fatalf("query payloads: %v", err)
			}
			if uint64(nonce) != tc.nonce {
				t.Errorf("nonce = %d, want %d", nonce, tc.nonce)
			}
		})
	}
}

func TestCrossProtocol_FindByCorrelationKey_NonceBased(t *testing.T) {
	store := newTestMessageStore(t)
	ctx := context.Background()

	// Insert LayerZero and Wormhole messages.
	lzMsg := newCrossProtocolMessage(crossProtocolCases[0]) // LayerZero
	whMsg := newCrossProtocolMessage(crossProtocolCases[1]) // Wormhole
	t.Cleanup(func() {
		cleanupMessage(t, store, lzMsg.MessageID)
		cleanupMessage(t, store, whMsg.MessageID)
	})

	if err := store.UpsertMessage(ctx, lzMsg); err != nil {
		t.Fatalf("upsert LZ: %v", err)
	}
	if err := store.UpsertMessage(ctx, whMsg); err != nil {
		t.Fatalf("upsert WH: %v", err)
	}

	// LayerZero key should find LayerZero message
	lzKey := &normalizer.CorrelationKey{
		Protocol:   "layerzero_v2",
		Nonce:      42,
		Sender:     lzMsg.Source.Sender,
		SrcChainID: 1,
	}
	found, err := store.FindByCorrelationKey(ctx, lzKey)
	if err != nil {
		t.Fatalf("FindByCorrelationKey LZ: %v", err)
	}
	if found == nil {
		t.Fatal("expected to find LayerZero message")
	}
	if found.MessageID != lzMsg.MessageID {
		t.Errorf("found MessageID = %q, want %q", found.MessageID, lzMsg.MessageID)
	}

	// Wormhole key should find Wormhole message
	whKey := &normalizer.CorrelationKey{
		Protocol:   "wormhole",
		Nonce:      42,
		Sender:     whMsg.Source.Sender,
		SrcChainID: 1,
	}
	found, err = store.FindByCorrelationKey(ctx, whKey)
	if err != nil {
		t.Fatalf("FindByCorrelationKey WH: %v", err)
	}
	if found == nil {
		t.Fatal("expected to find Wormhole message")
	}
	if found.MessageID != whMsg.MessageID {
		t.Errorf("found MessageID = %q, want %q", found.MessageID, whMsg.MessageID)
	}

	// Protocol isolation: LZ key with WH protocol should NOT match
	crossKey := &normalizer.CorrelationKey{
		Protocol:   "wormhole",
		Nonce:      42,
		Sender:     lzMsg.Source.Sender, // LZ sender, WH protocol
		SrcChainID: 1,
	}
	found, err = store.FindByCorrelationKey(ctx, crossKey)
	if err != nil {
		t.Fatalf("FindByCorrelationKey cross: %v", err)
	}
	if found != nil {
		t.Error("expected nil for cross-protocol key mismatch")
	}
}

func TestCrossProtocol_FindByCorrelationKey_MessageIDBased(t *testing.T) {
	store := newTestMessageStore(t)
	ctx := context.Background()

	// Insert Axelar and CCIP messages.
	axlMsg := newCrossProtocolMessage(crossProtocolCases[2])  // Axelar
	ccipMsg := newCrossProtocolMessage(crossProtocolCases[3]) // CCIP
	t.Cleanup(func() {
		cleanupMessage(t, store, axlMsg.MessageID)
		cleanupMessage(t, store, ccipMsg.MessageID)
	})

	if err := store.UpsertMessage(ctx, axlMsg); err != nil {
		t.Fatalf("upsert Axelar: %v", err)
	}
	if err := store.UpsertMessage(ctx, ccipMsg); err != nil {
		t.Fatalf("upsert CCIP: %v", err)
	}

	// Axelar key should find Axelar message
	axlKey := &normalizer.CorrelationKey{
		Protocol:  "axelar",
		MessageID: axlMsg.MessageID,
	}
	found, err := store.FindByCorrelationKey(ctx, axlKey)
	if err != nil {
		t.Fatalf("FindByCorrelationKey Axelar: %v", err)
	}
	if found == nil {
		t.Fatal("expected to find Axelar message")
	}
	if found.MessageID != axlMsg.MessageID {
		t.Errorf("found MessageID = %q, want %q", found.MessageID, axlMsg.MessageID)
	}

	// CCIP key should find CCIP message
	ccipKey := &normalizer.CorrelationKey{
		Protocol:  "ccip",
		MessageID: ccipMsg.MessageID,
	}
	found, err = store.FindByCorrelationKey(ctx, ccipKey)
	if err != nil {
		t.Fatalf("FindByCorrelationKey CCIP: %v", err)
	}
	if found == nil {
		t.Fatal("expected to find CCIP message")
	}
	if found.MessageID != ccipMsg.MessageID {
		t.Errorf("found MessageID = %q, want %q", found.MessageID, ccipMsg.MessageID)
	}

	// Protocol isolation: Axelar MessageID with CCIP protocol should NOT match
	crossKey := &normalizer.CorrelationKey{
		Protocol:  "ccip",
		MessageID: axlMsg.MessageID,
	}
	found, err = store.FindByCorrelationKey(ctx, crossKey)
	if err != nil {
		t.Fatalf("FindByCorrelationKey cross: %v", err)
	}
	if found != nil {
		t.Error("expected nil for cross-protocol MessageID mismatch")
	}
}

func TestCrossProtocol_FullCorrelationFlow_AllProtocols(t *testing.T) {
	store := newTestMessageStore(t)
	ctx := context.Background()

	flows := []struct {
		name       string
		tc         protocolTestCase
		key        *normalizer.CorrelationKey
		destStatus types.MessageStatus
	}{
		{
			name: "LayerZero",
			tc:   crossProtocolCases[0],
			key: &normalizer.CorrelationKey{
				Protocol:   "layerzero_v2",
				Nonce:      42,
				Sender:     crossProtocolCases[0].sender,
				SrcChainID: 1,
			},
			destStatus: types.StatusExecuted,
		},
		{
			name: "Wormhole",
			tc:   crossProtocolCases[1],
			key: &normalizer.CorrelationKey{
				Protocol:   "wormhole",
				Nonce:      42,
				Sender:     crossProtocolCases[1].sender,
				SrcChainID: 1,
			},
			destStatus: types.StatusExecuted,
		},
		{
			name: "Axelar",
			tc:   crossProtocolCases[2],
			key: &normalizer.CorrelationKey{
				Protocol:  "axelar",
				MessageID: crossProtocolCases[2].msgID,
			},
			destStatus: types.StatusExecuted,
		},
		{
			name: "CCIP_Success",
			tc:   crossProtocolCases[3],
			key: &normalizer.CorrelationKey{
				Protocol:  "ccip",
				MessageID: crossProtocolCases[3].msgID,
			},
			destStatus: types.StatusExecuted,
		},
		{
			name: "CCIP_Failure",
			tc: protocolTestCase{
				name:     "CCIP_Failure",
				msgID:    "xproto-ccip-fail-001",
				protocol: "ccip",
				msgType:  types.TypeMessage,
				chainID:  1,
				sender:   "0xSenderCCIPFail1111111111111111111111111111",
				nonce:    99,
			},
			key: &normalizer.CorrelationKey{
				Protocol:  "ccip",
				MessageID: "xproto-ccip-fail-001",
			},
			destStatus: types.StatusFailed,
		},
	}

	for _, flow := range flows {
		t.Run(flow.name, func(t *testing.T) {
			msg := newCrossProtocolMessage(flow.tc)
			t.Cleanup(func() { cleanupMessage(t, store, flow.tc.msgID) })

			// Step 1: Upsert source message (pending)
			if err := store.UpsertMessage(ctx, msg); err != nil {
				t.Fatalf("UpsertMessage: %v", err)
			}

			// Step 2: Find by correlation key
			found, err := store.FindByCorrelationKey(ctx, flow.key)
			if err != nil {
				t.Fatalf("FindByCorrelationKey: %v", err)
			}
			if found == nil {
				t.Fatal("expected to find pending message")
			}

			// Step 3: Update destination
			dest := &types.Destination{
				ChainID:     42161,
				TxHash:      "0x" + flow.tc.protocol + "_dest_tx",
				BlockNumber: 180000000,
				Timestamp:   1706000045,
				Receiver:    "0xReceiver2222222222222222222222222222222222",
				LogIndex:    3,
			}
			if err := store.UpdateDestination(ctx, found.MessageID, dest, flow.destStatus); err != nil {
				t.Fatalf("UpdateDestination: %v", err)
			}

			// Step 4: Verify final status in DB
			var status string
			err = store.pool.QueryRow(ctx,
				"SELECT status FROM messages WHERE message_id = $1", flow.tc.msgID,
			).Scan(&status)
			if err != nil {
				t.Fatalf("query status: %v", err)
			}
			if status != string(flow.destStatus) {
				t.Errorf("status = %q, want %q", status, flow.destStatus)
			}

			// Step 5: Verify destination row exists
			var destChainID int64
			err = store.pool.QueryRow(ctx,
				"SELECT chain_id FROM message_destinations WHERE message_id = $1", flow.tc.msgID,
			).Scan(&destChainID)
			if err != nil {
				t.Fatalf("query destination: %v", err)
			}
			if destChainID != 42161 {
				t.Errorf("dest chain_id = %d, want 42161", destChainID)
			}

			// Step 6: Message should no longer be findable as pending
			found, err = store.FindByCorrelationKey(ctx, flow.key)
			if err != nil {
				t.Fatalf("second FindByCorrelationKey: %v", err)
			}
			if found != nil {
				t.Error("expected nil after message is no longer pending")
			}
		})
	}
}

func TestCrossProtocol_UnifiedQuery(t *testing.T) {
	store := newTestMessageStore(t)
	ctx := context.Background()

	// Insert one message per protocol.
	msgIDs := make([]string, len(crossProtocolCases))
	for i, tc := range crossProtocolCases {
		msg := newCrossProtocolMessage(tc)
		msgIDs[i] = tc.msgID
		t.Cleanup(func() { cleanupMessage(t, store, tc.msgID) })

		if err := store.UpsertMessage(ctx, msg); err != nil {
			t.Fatalf("UpsertMessage %s: %v", tc.name, err)
		}
	}

	// Query all 4 in a single SELECT.
	rows, err := store.pool.Query(ctx,
		"SELECT message_id, protocol, type, status FROM messages WHERE message_id = ANY($1) ORDER BY protocol",
		msgIDs,
	)
	if err != nil {
		t.Fatalf("unified query: %v", err)
	}
	defer rows.Close()

	found := map[string]string{}
	for rows.Next() {
		var id, protocol, msgType, status string
		if err := rows.Scan(&id, &protocol, &msgType, &status); err != nil {
			t.Fatalf("scan: %v", err)
		}
		found[protocol] = id
	}

	if len(found) != 4 {
		t.Fatalf("expected 4 protocols, got %d: %v", len(found), found)
	}
	for _, want := range []string{"layerzero_v2", "wormhole", "axelar", "ccip"} {
		if _, ok := found[want]; !ok {
			t.Errorf("protocol %q not found in unified query", want)
		}
	}
}
