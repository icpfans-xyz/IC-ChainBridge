package dfinity

import (
	"encoding/hex"
	"fmt"
	"strings"
	"time"
)

var (
	MethodGetProposal      = "getProposal"
	MethodHasVotedProposal = "hasVotedOnProposal"
	MethodVoteProposal     = "voteProposal"
	MethodExecuteProposal  = "executeProposal"

	MethodDepositRecordLastIndex = "getDepositRecordLastIndex"
	MethodDepositRecord          = "getDepositRecordByIndex"
)

type Hash [32]byte

// NewHash creates a new Hash type
func NewHash(b []byte) Hash {
	h := Hash{}
	copy(h[:], b)
	return h
}

func HexDecodeString(s string) ([]byte, error) {
	s = strings.TrimPrefix(s, "0x")

	if len(s)%2 != 0 {
		s = "0" + s
	}

	b, err := hex.DecodeString(s)
	if err != nil {
		return nil, err
	}

	return b, nil
}

// NewHashFromHexString creates a new Hash type from a hex string
func NewHashFromHexString(s string) (Hash, error) {
	bz, err := HexDecodeString(s)
	if err != nil {
		return Hash{}, err
	}

	if len(bz) != 32 {
		return Hash{}, fmt.Errorf("required result to be 32 bytes, but got %v", len(bz))
	}

	return NewHash(bz), nil
}

func (h Hash) Hex() string {
	return fmt.Sprintf("%#x", h[:])
}

// Proposal represents an on-chain Proposal
type Proposal struct {
	depositNonce uint64
	sourceId     uint8
	resourceId   []byte
	Method       string
	Status       uint8
}

type DepositRecord struct {
	Index              uint64
	Caller             string
	DestinationChainID uint16
	DepositNonce       uint64
	ResourceID         string
	Depositer          string
	RecipientAddress   string
	Amount             uint64
	BridgeFee          uint64
	TokenFee           uint64
	Timestamp          time.Time
	TxID               string
	DepositResult      bool
	Error              string
}
