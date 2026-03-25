package normalizer

import (
	"testing"

	"github.com/Patrick-Ehimen/akave-crosschain-archive/internal/decoder"
	"github.com/Patrick-Ehimen/akave-crosschain-archive/internal/types"
)

// ─── Cross-protocol schema consistency tests ──────────────────────────────
//
// These tests verify that all 4 protocol decoders produce structurally
// consistent types.Message output through the normalizer, fulfilling the
// unified schema requirement of Milestone 3.

func TestAllProtocols_SourceEvents_ProduceConsistentSchema(t *testing.T) {
	tests := []struct {
		name     string
		event    *decoder.RawEvent
		protocol string
	}{
		{
			name:     "LayerZero/PacketSent",
			event:    newPacketSentEvent(),
			protocol: "layerzero_v2",
		},
		{
			name:     "Wormhole/LogMessagePublished",
			event:    newLogMessagePublishedEvent(),
			protocol: "wormhole",
		},
		{
			name:     "Axelar/ContractCallApproved",
			event:    newContractCallApprovedEvent(),
			protocol: "axelar",
		},
		{
			name:     "CCIP/CCIPSendRequested",
			event:    newCCIPSendRequestedEvent(),
			protocol: "ccip",
		},
	}

	validTypes := map[types.MessageType]bool{
		types.TypeMessage:       true,
		types.TypeTokenTransfer: true,
		types.TypeContractCall:  true,
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := Normalize(tt.event)
			if err != nil {
				t.Fatalf("Normalize() error: %v", err)
			}

			if result.IsCorrelation {
				t.Error("source event must not be a correlation")
			}

			msg := result.Message
			if msg == nil {
				t.Fatal("result.Message is nil")
			}

			// Protocol
			if msg.Protocol != tt.protocol {
				t.Errorf("Protocol = %q, want %q", msg.Protocol, tt.protocol)
			}

			// MessageID
			if msg.MessageID == "" {
				t.Error("MessageID is empty")
			}

			// Type
			if !validTypes[msg.Type] {
				t.Errorf("Type = %q, not a valid MessageType", msg.Type)
			}

			// Status
			if msg.Status != types.StatusPending {
				t.Errorf("Status = %q, want %q", msg.Status, types.StatusPending)
			}

			// Source fields
			if msg.Source.ChainID == 0 {
				t.Error("Source.ChainID is 0")
			}
			if msg.Source.TxHash == "" {
				t.Error("Source.TxHash is empty")
			}
			if msg.Source.BlockNumber == 0 {
				t.Error("Source.BlockNumber is 0")
			}
			if msg.Source.Timestamp == 0 {
				t.Error("Source.Timestamp is 0")
			}
			if msg.Source.Sender == "" {
				t.Error("Source.Sender is empty")
			}

			// Destination must be nil for source events
			if msg.Destination != nil {
				t.Error("Destination should be nil for source events")
			}

			// Payload must be populated
			if msg.Payload == nil {
				t.Error("Payload is nil")
			}
		})
	}
}

func TestAllProtocols_CorrelationEvents_ProduceConsistentResult(t *testing.T) {
	tests := []struct {
		name           string
		event          *decoder.RawEvent
		protocol       string
		usesMessageID  bool // true for Axelar/CCIP direct lookup
		usesNonceKey   bool // true for LayerZero/Wormhole tuple lookup
		expectReceiver bool // some protocols don't set receiver on correlation
	}{
		{
			name:           "LayerZero/PacketReceived",
			event:          newPacketReceivedEvent(),
			protocol:       "layerzero_v2",
			usesNonceKey:   true,
			expectReceiver: true,
		},
		{
			name:           "Wormhole/TransferRedeemed",
			event:          newTransferRedeemedEvent(),
			protocol:       "wormhole",
			usesNonceKey:   true,
			expectReceiver: true,
		},
		{
			name:         "Axelar/Executed",
			event:        newExecutedEvent(),
			protocol:     "axelar",
			usesMessageID: true,
		},
		{
			name:         "CCIP/ExecutionStateChanged",
			event:        newCCIPExecutionStateChangedEvent(),
			protocol:     "ccip",
			usesMessageID: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := Normalize(tt.event)
			if err != nil {
				t.Fatalf("Normalize() error: %v", err)
			}

			if !result.IsCorrelation {
				t.Error("correlation event must have IsCorrelation=true")
			}

			// CorrelationKey
			key := result.CorrelationKey
			if key == nil {
				t.Fatal("CorrelationKey is nil")
			}
			if key.Protocol != tt.protocol {
				t.Errorf("CorrelationKey.Protocol = %q, want %q", key.Protocol, tt.protocol)
			}

			// Validate key type
			if tt.usesMessageID {
				if key.MessageID == "" {
					t.Error("MessageID-based correlation has empty MessageID")
				}
			}
			if tt.usesNonceKey {
				if key.Nonce == 0 {
					t.Error("nonce-based correlation has zero Nonce")
				}
				if key.Sender == "" {
					t.Error("nonce-based correlation has empty Sender")
				}
				if key.SrcChainID == 0 {
					t.Error("nonce-based correlation has zero SrcChainID")
				}
			}

			// Destination
			msg := result.Message
			if msg == nil {
				t.Fatal("result.Message is nil")
			}
			if msg.Destination == nil {
				t.Fatal("Destination is nil for correlation event")
			}
			if msg.Destination.ChainID == 0 {
				t.Error("Destination.ChainID is 0")
			}
			if msg.Destination.TxHash == "" {
				t.Error("Destination.TxHash is empty")
			}
			if msg.Destination.BlockNumber == 0 {
				t.Error("Destination.BlockNumber is 0")
			}
			if msg.Destination.Timestamp == 0 {
				t.Error("Destination.Timestamp is 0")
			}
		})
	}
}

func TestAllProtocols_ProtocolNames(t *testing.T) {
	expected := map[string]*decoder.RawEvent{
		"layerzero_v2": newPacketSentEvent(),
		"wormhole":     newLogMessagePublishedEvent(),
		"axelar":       newContractCallApprovedEvent(),
		"ccip":         newCCIPSendRequestedEvent(),
	}

	for wantProtocol, event := range expected {
		t.Run(wantProtocol, func(t *testing.T) {
			result, err := Normalize(event)
			if err != nil {
				t.Fatalf("Normalize() error: %v", err)
			}
			if result.Message.Protocol != wantProtocol {
				t.Errorf("Protocol = %q, want %q", result.Message.Protocol, wantProtocol)
			}
		})
	}
}

func TestCCIP_FailureStatus_PreservedInCorrelation(t *testing.T) {
	event := newCCIPExecutionFailedEvent()
	result, err := Normalize(event)
	if err != nil {
		t.Fatalf("Normalize() error: %v", err)
	}

	if !result.IsCorrelation {
		t.Error("ExecutionStateChanged should be a correlation event")
	}
	if result.Message.Status != types.StatusFailed {
		t.Errorf("Status = %q, want %q for state=3", result.Message.Status, types.StatusFailed)
	}
}
