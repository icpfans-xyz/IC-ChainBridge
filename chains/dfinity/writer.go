package dfinity

import (
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"math/big"

	"github.com/ChainSafe/chainbridge-utils/core"
	metrics "github.com/ChainSafe/chainbridge-utils/metrics/types"
	"github.com/ChainSafe/chainbridge-utils/msg"
	"github.com/ChainSafe/log15"
)

var _ core.Writer = &writer{}

// https://github.com/ChainSafe/chainbridge-solidity/blob/b5ed13d9798feb7c340e737a726dd415b8815366/contracts/Bridge.sol#L20
var PassedStatus uint8 = 2
var TransferredStatus uint8 = 3
var CancelledStatus uint8 = 4

type writer struct {
	chainId       msg.ChainId
	conn          *Connection
	log           log15.Logger
	erc20Canister string
	stop          <-chan int
	sysErr        chan<- error
	metrics       *metrics.ChainMetrics
	extendCall    bool // Extend extrinsic calls to substrate with ResourceID.Used for backward compatibility with example pallet.
}

func NewWriter(id msg.ChainId, conn *Connection, log log15.Logger, erc20Canister string, stop <-chan int, sysErr chan<- error, m *metrics.ChainMetrics, extendCall bool) *writer {
	return &writer{
		chainId:       id,
		conn:          conn,
		log:           log,
		erc20Canister: erc20Canister,
		stop:          stop,
		sysErr:        sysErr,
		metrics:       m,
		extendCall:    extendCall,
	}
}

func (w *writer) ResolveMessage(m msg.Message) bool {
	w.log.Info("Attempting to resolve message", "type", m.Type, "src", m.Source, "dst", m.Destination, "nonce", m.DepositNonce, "rId", m.ResourceId.Hex())

	switch m.Type {
	case msg.FungibleTransfer:
		return w.createFungibleProposal(m)
	case msg.NonFungibleTransfer:
		return w.createNonFungibleProposal(m)
	case msg.GenericTransfer:
		return w.createGenericProposal(m)
	default:
		w.log.Error("Unknown message type received", "type", m.Type)
		return false
	}
}

// Sha256 returns the SHA-256 digest of the data.
func Sha256(args ...[]byte) []byte {
	hasher := sha256.New()
	for _, bytes := range args {
		hasher.Write(bytes)
	}
	return hasher.Sum(nil)
}

// FromUint16 decodes uint16.
func FromUint16(v uint16) []byte {
	b := make([]byte, 2)
	binary.BigEndian.PutUint16(b, v)
	return b
}

// FromUint64 decodes unit64 value.
func FromUint64(v uint64) []byte {
	b := make([]byte, 8)
	binary.BigEndian.PutUint64(b, v)
	return b
}

func (w *writer) calculateDataHash(m msg.Message) string {
	// let handlerAddressBytes = Blob.toArray(Text.encodeUtf8(handlerAddress));
	// let dataBytes = Blob.toArray(Text.encodeUtf8(data.recipientAddress # Nat.toText(data.amount)));
	// let bytes : [Nat8]= Array.flatten([ Nat64Ext.toNat8Array(depositNonce),
	// 									Nat16Ext.toNat8Array(chainID),
	// 									handlerAddressBytes, dataBytes]);

	// let dataHash = SHA256.encode(SHA256.sha256(bytes));
	amount := big.NewInt(0).SetBytes(m.Payload[0].([]byte))
	recipient := string(m.Payload[1].([]byte))
	dataStr := fmt.Sprintf("%s%d", recipient, amount.Int64())
	dataBytes := []byte(dataStr)
	bytes := append(FromUint64(uint64(m.DepositNonce)), FromUint16(uint16(w.chainId))...)
	bytes = append(bytes, []byte(w.erc20Canister)...)
	bytes = append(bytes, dataBytes...)
	shaBytes := Sha256(bytes)

	return string(shaBytes)
}
