package archiver

import (
	"bytes"
	"fmt"
	"testing"
	"time"
)

func TestWriteParquet_RoundTrip(t *testing.T) {
	dstChainID := uint64(42161)
	dstTxHash := "0xdef456"
	dstBlockNum := uint64(180000000)
	dstTimestamp := int64(1706000045)
	dstReceiver := "0xReceiver"
	dstLogIndex := int32(3)
	payloadToken := "0xToken"
	payloadAmount := "1000000"
	payloadData := "0xdeadbeef"
	payloadNonce := uint64(42)
	metaFee := "0.001"
	metaRelayer := "0xRelayer"
	metaGasUsed := uint64(150000)
	metaLatency := int64(45)

	records := []ParquetRecord{
		{
			MessageID:          "guid-001",
			Protocol:           "layerzero_v2",
			Type:               "message",
			Status:             "executed",
			CreatedAt:          time.Date(2025, 1, 15, 10, 0, 0, 0, time.UTC),
			UpdatedAt:          time.Date(2025, 1, 15, 10, 1, 0, 0, time.UTC),
			SrcChainID:         1,
			SrcTxHash:          "0xabc123",
			SrcBlockNumber:     19000000,
			SrcTimestamp:       1706000000,
			SrcSender:          "0xSender",
			SrcLogIndex:        5,
			DstChainID:         &dstChainID,
			DstTxHash:          &dstTxHash,
			DstBlockNumber:     &dstBlockNum,
			DstTimestamp:       &dstTimestamp,
			DstReceiver:        &dstReceiver,
			DstLogIndex:        &dstLogIndex,
			PayloadToken:       &payloadToken,
			PayloadAmount:      &payloadAmount,
			PayloadData:        &payloadData,
			PayloadNonce:       &payloadNonce,
			MetaFee:            &metaFee,
			MetaRelayer:        &metaRelayer,
			MetaGasUsed:        &metaGasUsed,
			MetaLatencySeconds: &metaLatency,
		},
	}

	buf, err := WriteParquet(records)
	if err != nil {
		t.Fatalf("WriteParquet failed: %v", err)
	}

	if buf.Len() == 0 {
		t.Fatal("expected non-empty parquet output")
	}

	reader := bytes.NewReader(buf.Bytes())
	got, err := ReadParquet(reader, int64(buf.Len()))
	if err != nil {
		t.Fatalf("ReadParquet failed: %v", err)
	}

	if len(got) != 1 {
		t.Fatalf("expected 1 record, got %d", len(got))
	}

	r := got[0]
	if r.MessageID != "guid-001" {
		t.Errorf("expected MessageID guid-001, got %s", r.MessageID)
	}
	if r.Protocol != "layerzero_v2" {
		t.Errorf("expected Protocol layerzero_v2, got %s", r.Protocol)
	}
	if r.SrcChainID != 1 {
		t.Errorf("expected SrcChainID 1, got %d", r.SrcChainID)
	}
	if r.DstChainID == nil || *r.DstChainID != 42161 {
		t.Errorf("expected DstChainID 42161, got %v", r.DstChainID)
	}
	if r.PayloadNonce == nil || *r.PayloadNonce != 42 {
		t.Errorf("expected PayloadNonce 42, got %v", r.PayloadNonce)
	}
	if r.MetaLatencySeconds == nil || *r.MetaLatencySeconds != 45 {
		t.Errorf("expected MetaLatencySeconds 45, got %v", r.MetaLatencySeconds)
	}
}

func TestWriteParquet_NilOptionalFields(t *testing.T) {
	records := []ParquetRecord{
		{
			MessageID:      "guid-pending",
			Protocol:       "layerzero_v2",
			Type:           "message",
			Status:         "pending",
			CreatedAt:      time.Now().UTC(),
			UpdatedAt:      time.Now().UTC(),
			SrcChainID:     1,
			SrcTxHash:      "0xpending",
			SrcBlockNumber: 19000000,
			SrcTimestamp:   1706000000,
			SrcSender:      "0xSender",
			SrcLogIndex:    0,
		},
	}

	buf, err := WriteParquet(records)
	if err != nil {
		t.Fatalf("WriteParquet failed: %v", err)
	}

	got, err := ReadParquet(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	if err != nil {
		t.Fatalf("ReadParquet failed: %v", err)
	}

	if len(got) != 1 {
		t.Fatalf("expected 1 record, got %d", len(got))
	}

	r := got[0]
	if r.DstChainID != nil {
		t.Errorf("expected nil DstChainID, got %v", r.DstChainID)
	}
	if r.PayloadToken != nil {
		t.Errorf("expected nil PayloadToken, got %v", r.PayloadToken)
	}
	if r.MetaFee != nil {
		t.Errorf("expected nil MetaFee, got %v", r.MetaFee)
	}
}

func TestWriteParquet_MultipleRecords(t *testing.T) {
	records := make([]ParquetRecord, 100)
	for i := range records {
		records[i] = ParquetRecord{
			MessageID:      fmt.Sprintf("guid-%03d", i),
			Protocol:       "layerzero_v2",
			Type:           "message",
			Status:         "pending",
			CreatedAt:      time.Now().UTC(),
			UpdatedAt:      time.Now().UTC(),
			SrcChainID:     1,
			SrcTxHash:      fmt.Sprintf("0xtx%03d", i),
			SrcBlockNumber: uint64(19000000 + i),
			SrcTimestamp:   int64(1706000000 + i),
			SrcSender:      "0xSender",
			SrcLogIndex:    int32(i),
		}
	}

	buf, err := WriteParquet(records)
	if err != nil {
		t.Fatalf("WriteParquet failed: %v", err)
	}

	got, err := ReadParquet(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	if err != nil {
		t.Fatalf("ReadParquet failed: %v", err)
	}

	if len(got) != 100 {
		t.Fatalf("expected 100 records, got %d", len(got))
	}
}

func TestWriteParquet_EmptySlice(t *testing.T) {
	_, err := WriteParquet(nil)
	if err == nil {
		t.Fatal("expected error for nil records")
	}

	_, err = WriteParquet([]ParquetRecord{})
	if err == nil {
		t.Fatal("expected error for empty records")
	}
}
