# Protocol Addition Guide

This guide walks through adding a new cross-chain protocol (e.g. Hyperlane, Stargate) to CrossChain Archive.

## Overview

Adding a new protocol requires:
1. Implementing the `Decoder` interface
2. Adding contract addresses and event ABIs
3. Implementing a `Normalizer` result for the new event types
4. Registering the decoder in `cmd/` entry points
5. Writing tests

---

## Step 1: Create the Decoder Package

Create a new package under `internal/decoder/<protocol>/`:

```
internal/decoder/myprotocol/
├── decoder.go      # Decoder interface implementation
├── abi.go          # ABI definitions and parsed types
└── decoder_test.go # Unit tests
```

### 1a. Define the ABI (`abi.go`)

```go
package myprotocol

import (
    "github.com/ethereum/go-ethereum/accounts/abi"
    "strings"
)

// MessageSentABI is the ABI fragment for the MessageSent event.
const messageSentABIJSON = `[{
    "type": "event",
    "name": "MessageSent",
    "inputs": [
        {"name": "messageId", "type": "bytes32", "indexed": true},
        {"name": "origin",    "type": "uint32",  "indexed": false},
        {"name": "destination","type": "uint32", "indexed": false},
        {"name": "sender",    "type": "bytes32", "indexed": false},
        {"name": "recipient", "type": "bytes32", "indexed": false},
        {"name": "body",      "type": "bytes",   "indexed": false}
    ]
}]`

var messageSentABI abi.ABI

func init() {
    var err error
    messageSentABI, err = abi.JSON(strings.NewReader(messageSentABIJSON))
    if err != nil {
        panic("myprotocol: invalid ABI: " + err.Error())
    }
}
```

### 1b. Implement the Decoder (`decoder.go`)

```go
package myprotocol

import (
    "fmt"

    "github.com/ethereum/go-ethereum/common"
    ethtypes "github.com/ethereum/go-ethereum/core/types"

    "github.com/Patrick-Ehimen/akave-crosschain-archive/internal/decoder"
)

const Protocol = "myprotocol"

// contractsByChain maps chain IDs to the contracts that emit events for this protocol.
var contractsByChain = map[uint64][]common.Address{
    1:     {common.HexToAddress("0xMailboxAddress1")},
    42161: {common.HexToAddress("0xMailboxAddressArb")},
}

// Decoder implements decoder.Decoder for MyProtocol.
type Decoder struct{}

func New() *Decoder { return &Decoder{} }

// Protocol returns the unique protocol identifier.
func (d *Decoder) Protocol() string { return Protocol }

// ContractAddresses returns the mailbox contract addresses for the given chain.
func (d *Decoder) ContractAddresses(chainID uint64) []common.Address {
    return contractsByChain[chainID]
}

// EventTopics returns the event topic hashes this decoder handles.
func (d *Decoder) EventTopics() []common.Hash {
    return []common.Hash{
        messageSentABI.Events["MessageSent"].ID,
    }
}

// Decode parses a raw Ethereum log into a RawEvent.
func (d *Decoder) Decode(log ethtypes.Log, chainID uint64) (*decoder.RawEvent, error) {
    if len(log.Topics) == 0 {
        return nil, fmt.Errorf("myprotocol: empty topics")
    }

    switch log.Topics[0] {
    case messageSentABI.Events["MessageSent"].ID:
        return d.decodeMessageSent(log, chainID)
    default:
        return nil, fmt.Errorf("myprotocol: unknown topic %s", log.Topics[0])
    }
}

func (d *Decoder) decodeMessageSent(log ethtypes.Log, chainID uint64) (*decoder.RawEvent, error) {
    // Indexed fields come from Topics; non-indexed from Data.
    if len(log.Topics) < 2 {
        return nil, fmt.Errorf("myprotocol: MessageSent: not enough topics")
    }

    messageID := log.Topics[1].Hex()

    var parsed struct {
        Origin      uint32
        Destination uint32
        Sender      [32]byte
        Recipient   [32]byte
        Body        []byte
    }
    if err := messageSentABI.UnpackIntoInterface(&parsed, "MessageSent", log.Data); err != nil {
        return nil, fmt.Errorf("myprotocol: unpack MessageSent: %w", err)
    }

    return &decoder.RawEvent{
        Protocol:    Protocol,
        ChainID:     chainID,
        BlockNumber: log.BlockNumber,
        TxHash:      log.TxHash.Hex(),
        LogIndex:    log.Index,
        EventType:   "MessageSent",
        Data: map[string]string{
            "message_id":  messageID,
            "origin":      fmt.Sprintf("%d", parsed.Origin),
            "destination": fmt.Sprintf("%d", parsed.Destination),
            "sender":      common.BytesToAddress(parsed.Sender[:]).Hex(),
            "recipient":   common.BytesToAddress(parsed.Recipient[:]).Hex(),
        },
    }, nil
}
```

---

## Step 2: Add Normalization Support

The normalizer (`internal/normalizer/normalizer.go`) converts a `RawEvent` into a `Result` (either a `Message` or a `CorrelationKey`). Add a case for your protocol:

```go
case "myprotocol":
    return n.normalizeMyProtocol(event)
```

Implement the function:

```go
func (n *Normalizer) normalizeMyProtocol(event *decoder.RawEvent) (*Result, error) {
    switch event.EventType {
    case "MessageSent":
        msgID := fmt.Sprintf("myprotocol-%s", event.Data["message_id"])
        return &Result{
            Message: &types.Message{
                MessageID: msgID,
                Protocol:  event.Protocol,
                Type:      types.MessageTypeMessage,
                Status:    types.StatusPending,
                Source: types.MessageSource{
                    ChainID:     event.ChainID,
                    TxHash:      event.TxHash,
                    BlockNumber: event.BlockNumber,
                    Timestamp:   event.Timestamp,
                    Sender:      event.Data["sender"],
                    LogIndex:    int(event.LogIndex),
                },
            },
        }, nil

    case "MessageDelivered":
        // Return a CorrelationKey so the correlator can match to an existing pending message.
        msgID := fmt.Sprintf("myprotocol-%s", event.Data["message_id"])
        return &Result{
            CorrelationKey: &types.CorrelationKey{
                MessageID: msgID,
                Destination: &types.MessageDestination{
                    ChainID:     event.ChainID,
                    TxHash:      event.TxHash,
                    BlockNumber: event.BlockNumber,
                    Timestamp:   event.Timestamp,
                    Receiver:    event.Data["recipient"],
                    LogIndex:    int(event.LogIndex),
                },
            },
        }, nil
    }
    return nil, fmt.Errorf("myprotocol: unknown event type %s", event.EventType)
}
```

---

## Step 3: Register the Decoder

In each binary entrypoint, register your decoder alongside the others:

**`cmd/indexer/main.go`** and **`cmd/backfill/main.go`**:

```go
import "github.com/Patrick-Ehimen/akave-crosschain-archive/internal/decoder/myprotocol"

// ...

registry.Register(myprotocol.New())
```

---

## Step 4: Add Contract Addresses

Add your protocol's contracts to the chain configs in `internal/decoder/myprotocol/decoder.go` or load them from config if they vary per deployment.

---

## Step 5: Write Tests

Create `internal/decoder/myprotocol/decoder_test.go`:

```go
package myprotocol_test

import (
    "math/big"
    "testing"

    "github.com/ethereum/go-ethereum/common"
    ethtypes "github.com/ethereum/go-ethereum/core/types"
    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"

    "github.com/Patrick-Ehimen/akave-crosschain-archive/internal/decoder/myprotocol"
)

func TestDecodeMessageSent(t *testing.T) {
    d := myprotocol.New()

    // Build a realistic log using ABI encoding (see existing decoder tests for patterns).
    log := ethtypes.Log{
        BlockNumber: 1000,
        TxHash:      common.HexToHash("0xabc"),
        Index:       0,
        Topics: []common.Hash{
            // MessageSent event ID
            d.EventTopics()[0],
            // messageId (indexed)
            common.HexToHash("0x" + "01" + strings.Repeat("00", 31)),
        },
        Data: buildMessageSentData(t, 1, 42161, "0xSender", "0xRecipient"),
    }

    event, err := d.Decode(log, 1)
    require.NoError(t, err)
    assert.Equal(t, "myprotocol", event.Protocol)
    assert.Equal(t, "MessageSent", event.EventType)
    assert.NotEmpty(t, event.Data["message_id"])
}
```

Use ABI encoding to build `log.Data` (see `internal/decoder/layerzero/decoder_test.go` for a complete example).

---

## Correlation Key Strategies

| Pattern | When to use | Example |
|---------|-------------|---------|
| `MessageID` direct lookup | Source and destination share a unique ID | Axelar `commandId`, CCIP `messageId`, MyProtocol `messageId` |
| Composite key | No single shared ID | LayerZero GUID, Wormhole `(emitterChain, emitterAddr, sequence)` |

The correlator in `internal/correlator/correlator.go` handles both via the `CorrelationKey` struct.

---

## Checklist

- [ ] `internal/decoder/<protocol>/decoder.go` — implements `decoder.Decoder`
- [ ] `internal/decoder/<protocol>/abi.go` — ABI definitions
- [ ] `internal/decoder/<protocol>/decoder_test.go` — unit tests with ABI-encoded logs
- [ ] `internal/normalizer/normalizer.go` — normalization case added
- [ ] `cmd/indexer/main.go` — decoder registered
- [ ] `cmd/backfill/main.go` — decoder registered
- [ ] Contract addresses added for all supported chains
- [ ] Decoder coverage remains >85% (`make coverage`)
