package ethereum

import (
	"strings"

	emitter "github.com/ChainSafe/ChainBridgeV2/contracts/Emitter"
	msg "github.com/ChainSafe/ChainBridgeV2/message"
	"github.com/ChainSafe/log15"
	"github.com/ethereum/go-ethereum/accounts/abi"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	ethcrypto "github.com/ethereum/go-ethereum/crypto"
)

func (l *Listener) handleTransferEvent(eventI interface{}) msg.Message {
	log15.Debug("Handling deposit proposal event")
	event := eventI.(ethtypes.Log)

	contractAbi, err := abi.JSON(strings.NewReader(string(emitter.EmitterABI)))
	if err != nil {
		log15.Error("Unable to decode event", err)
	}

	var nftEvent emitter.EmitterNFTTransfer
	err = contractAbi.Unpack(&nftEvent, "NFTTransfer", event.Data)
	if err != nil {
		log15.Error("Unable to unpack NFTTransfer", err)
	}

	// Capture indexed values
	nftEvent.DestChain = event.Topics[1].Big()
	nftEvent.DepositId = event.Topics[2].Big()

	msg := msg.Message{
		Type:        msg.CreateDepositProposalType,
		Destination: msg.ChainId(uint8(nftEvent.DestChain.Uint64())),
		Data:        nftEvent.Data,
	}
	msg.EncodeCreateDepositProposalData(nftEvent.DepositId, l.cfg.id)

	return msg
}

func (l *Listener) handleTestDeposit(eventI interface{}) msg.Message {
	event := eventI.(ethtypes.Log)
	data := ethcrypto.Keccak256Hash(event.Topics[0].Bytes()).Bytes()
	return msg.Message{
		Type: msg.DepositAssetType,
		Data: data,
	}
}