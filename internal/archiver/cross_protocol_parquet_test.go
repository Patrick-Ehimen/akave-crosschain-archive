package archiver

import (
	"bytes"
	"fmt"
	"testing"
	"time"
)

// ─── Cross-protocol Parquet tests ─────────────────────────────────────────
//
// These tests verify that Parquet serialization round-trips correctly for
// all 4 protocols and that the archival object key format is correct.

func newCrossProtocolRecords() []ParquetRecord {
	dstChainID := uint64(42161)
	dstTxHash := "0xdest_tx"
	dstBlockNum := uint64(180000000)
	dstTimestamp := int64(1706000045)
	dstReceiver := "0xReceiver"
	dstLogIndex := int32(3)

	payloadData := "0xdeadbeef"
	payloadNonce := uint64(42)
	payloadToken := "0xA0b86991c6218b36c1d19D4a2e9Eb0cE3606eB48"
	payloadAmount := "1000000000"

	metaFee := "100000000000000"
	metaRelayer := "0xRelayer"
	metaGasUsed := uint64(200000)
	metaLatency := int64(45)

	return []ParquetRecord{
		{
			MessageID:          "0x1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef",
			Protocol:           "layerzero_v2",
			Type:               "message",
			Status:             "executed",
			CreatedAt:          time.Date(2025, 1, 15, 10, 0, 0, 0, time.UTC),
			UpdatedAt:          time.Date(2025, 1, 15, 10, 1, 0, 0, time.UTC),
			SrcChainID:         1,
			SrcTxHash:          "0xlz_src_tx",
			SrcBlockNumber:     19000000,
			SrcTimestamp:       1706000000,
			SrcSender:          "0xSender1111111111111111111111111111111111",
			SrcLogIndex:        5,
			DstChainID:         &dstChainID,
			DstTxHash:          &dstTxHash,
			DstBlockNumber:     &dstBlockNum,
			DstTimestamp:       &dstTimestamp,
			DstReceiver:        &dstReceiver,
			DstLogIndex:        &dstLogIndex,
			PayloadData:        &payloadData,
			PayloadNonce:       &payloadNonce,
			MetaRelayer:        &metaRelayer,
			MetaGasUsed:        &metaGasUsed,
			MetaLatencySeconds: &metaLatency,
		},
		{
			MessageID:      "2/0x0000000000000000000000003ee18b2214aff97000d974cf647e7c347e8fa585/42",
			Protocol:       "wormhole",
			Type:           "message",
			Status:         "executed",
			CreatedAt:      time.Date(2025, 1, 15, 11, 0, 0, 0, time.UTC),
			UpdatedAt:      time.Date(2025, 1, 15, 11, 1, 0, 0, time.UTC),
			SrcChainID:     1,
			SrcTxHash:      "0xwh_src_tx",
			SrcBlockNumber: 19000100,
			SrcTimestamp:   1706000100,
			SrcSender:      "0x3ee18B2214AFF97000D974cf647E7C347E8fa585",
			SrcLogIndex:    7,
			DstChainID:     &dstChainID,
			DstTxHash:      &dstTxHash,
			DstBlockNumber: &dstBlockNum,
			DstTimestamp:   &dstTimestamp,
			DstReceiver:    &dstReceiver,
			DstLogIndex:    &dstLogIndex,
			PayloadData:    &payloadData,
			PayloadNonce:   &payloadNonce,
		},
		{
			MessageID:      "0xbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
			Protocol:       "axelar",
			Type:           "contract_call",
			Status:         "executed",
			CreatedAt:      time.Date(2025, 1, 15, 12, 0, 0, 0, time.UTC),
			UpdatedAt:      time.Date(2025, 1, 15, 12, 1, 0, 0, time.UTC),
			SrcChainID:     1,
			SrcTxHash:      "0xaxl_src_tx",
			SrcBlockNumber: 19500000,
			SrcTimestamp:   1707000000,
			SrcSender:      "0xSender1111111111111111111111111111111111",
			SrcLogIndex:    3,
			DstChainID:     &dstChainID,
			DstTxHash:      &dstTxHash,
			DstBlockNumber: &dstBlockNum,
			DstTimestamp:   &dstTimestamp,
			DstReceiver:    &dstReceiver,
			DstLogIndex:    &dstLogIndex,
		},
		{
			MessageID:      "0xabcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890ab",
			Protocol:       "ccip",
			Type:           "token_transfer",
			Status:         "executed",
			CreatedAt:      time.Date(2025, 1, 15, 13, 0, 0, 0, time.UTC),
			UpdatedAt:      time.Date(2025, 1, 15, 13, 1, 0, 0, time.UTC),
			SrcChainID:     1,
			SrcTxHash:      "0xccip_src_tx",
			SrcBlockNumber: 20000000,
			SrcTimestamp:   1708000000,
			SrcSender:      "0xSender1111111111111111111111111111111111",
			SrcLogIndex:    1,
			DstChainID:     &dstChainID,
			DstTxHash:      &dstTxHash,
			DstBlockNumber: &dstBlockNum,
			DstTimestamp:   &dstTimestamp,
			DstReceiver:    &dstReceiver,
			DstLogIndex:    &dstLogIndex,
			PayloadToken:   &payloadToken,
			PayloadAmount:  &payloadAmount,
			PayloadNonce:   &payloadNonce,
			MetaFee:        &metaFee,
		},
	}
}

func TestCrossProtocol_ParquetRoundTrip_AllProtocols(t *testing.T) {
	records := newCrossProtocolRecords()

	buf, err := WriteParquet(records)
	if err != nil {
		t.Fatalf("WriteParquet failed: %v", err)
	}
	if buf.Len() == 0 {
		t.Fatal("Parquet buffer is empty")
	}

	read, err := ReadParquet(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	if err != nil {
		t.Fatalf("ReadParquet failed: %v", err)
	}
	if len(read) != 4 {
		t.Fatalf("expected 4 records, got %d", len(read))
	}

	// Verify each protocol's record survived the round-trip.
	protocols := map[string]bool{}
	for _, r := range read {
		protocols[r.Protocol] = true
	}

	for _, want := range []string{"layerzero_v2", "wormhole", "axelar", "ccip"} {
		if !protocols[want] {
			t.Errorf("protocol %q missing from round-trip output", want)
		}
	}

	// Spot-check protocol-specific fields.
	for _, r := range read {
		if r.MessageID == "" {
			t.Error("MessageID is empty after round-trip")
		}
		if r.SrcChainID == 0 {
			t.Errorf("SrcChainID is 0 for %s", r.Protocol)
		}
		if r.DstChainID == nil || *r.DstChainID == 0 {
			t.Errorf("DstChainID missing for %s", r.Protocol)
		}

		switch r.Protocol {
		case "ccip":
			if r.PayloadToken == nil || *r.PayloadToken == "" {
				t.Error("CCIP record missing PayloadToken")
			}
			if r.MetaFee == nil || *r.MetaFee == "" {
				t.Error("CCIP record missing MetaFee")
			}
		case "axelar":
			if r.Type != "contract_call" {
				t.Errorf("Axelar record Type = %q, want contract_call", r.Type)
			}
		case "layerzero_v2":
			if r.MetaRelayer == nil || *r.MetaRelayer == "" {
				t.Error("LayerZero record missing MetaRelayer")
			}
		}
	}
}

func TestCrossProtocol_ParquetRoundTrip_PerProtocolFiles(t *testing.T) {
	records := newCrossProtocolRecords()

	for _, rec := range records {
		t.Run(rec.Protocol, func(t *testing.T) {
			buf, err := WriteParquet([]ParquetRecord{rec})
			if err != nil {
				t.Fatalf("WriteParquet failed for %s: %v", rec.Protocol, err)
			}

			read, err := ReadParquet(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
			if err != nil {
				t.Fatalf("ReadParquet failed for %s: %v", rec.Protocol, err)
			}
			if len(read) != 1 {
				t.Fatalf("expected 1 record, got %d", len(read))
			}
			if read[0].Protocol != rec.Protocol {
				t.Errorf("Protocol = %q, want %q", read[0].Protocol, rec.Protocol)
			}
			if read[0].MessageID != rec.MessageID {
				t.Errorf("MessageID = %q, want %q", read[0].MessageID, rec.MessageID)
			}
		})
	}
}

func TestCrossProtocol_ObjectKeyFormat(t *testing.T) {
	tests := []struct {
		protocol  string
		chainName string
		yearMonth string
		wantKey   string
	}{
		{"layerzero_v2", "ethereum", "2025-01", "protocols/layerzero_v2/ethereum/2025-01.parquet"},
		{"wormhole", "arbitrum", "2025-02", "protocols/wormhole/arbitrum/2025-02.parquet"},
		{"axelar", "avalanche", "2025-03", "protocols/axelar/avalanche/2025-03.parquet"},
		{"ccip", "base", "2025-01", "protocols/ccip/base/2025-01.parquet"},
		{"layerzero_v2", "polygon", "2024-12", "protocols/layerzero_v2/polygon/2024-12.parquet"},
	}

	for _, tt := range tests {
		t.Run(tt.wantKey, func(t *testing.T) {
			key := fmt.Sprintf("protocols/%s/%s/%s.parquet", tt.protocol, tt.chainName, tt.yearMonth)
			if key != tt.wantKey {
				t.Errorf("key = %q, want %q", key, tt.wantKey)
			}
		})
	}
}
