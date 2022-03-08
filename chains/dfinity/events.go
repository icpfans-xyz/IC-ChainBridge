package dfinity

import (
	"fmt"
	"math/big"

	"github.com/ChainSafe/chainbridge-utils/msg"
)

func (l *listener) HandleFungibleTransfer(evt *DepositRecord) (msg.Message, error) {

	rId, err := NewHashFromHexString(evt.ResourceID)
	if err != nil {
		return msg.Message{}, err
	}
	resourceId := msg.ResourceId(rId)
	l.log.Info("Got fungible transfer event!", "destination", evt.DestinationChainID, "resourceId", resourceId.Hex(), "amount", evt.Amount)

	return msg.NewFungibleTransfer(
		l.chainId, // Unset
		msg.ChainId(evt.DestinationChainID),
		msg.Nonce(evt.DepositNonce),
		big.NewInt(0).SetUint64(evt.Amount),
		resourceId,
		[]byte(evt.RecipientAddress),
	), nil
}

func (l *listener) HandleNonFungibleTransferHandler(evtI interface{}) (msg.Message, error) {
	return msg.Message{}, fmt.Errorf("ICP not support")

	// evt, ok := evtI.(*DepositRecord)
	// if !ok {
	// 	return msg.Message{}, fmt.Errorf("failed to cast EventNonFungibleTransfer type")
	// }

	// l.log.Info("Got non-fungible transfer event!", "destination", evt.DestinationChainID, "resourceId", evt.ResourceID)

	// return msg.NewNonFungibleTransfer(
	// 	l.chainId, // Unset
	// 	msg.ChainId(evt.DestinationChainID),
	// 	msg.Nonce(evt.DepositNonce),
	// 	msg.ResourceId(evt.ResourceID),
	// 	big.NewInt(0).SetBytes(nil),
	// 	nil,
	// 	nil,
	// ), nil
}

func (l *listener) HandleGenericTransferHandler(evtI interface{}) (msg.Message, error) {
	return msg.Message{}, fmt.Errorf("ICP not support")

	// evt, ok := evtI.(*DepositRecord)
	// if !ok {
	// 	return msg.Message{}, fmt.Errorf("failed to cast EventGenericTransfer type")
	// }

	// l.log.Info("Got generic transfer event!", "destination", evt.DestinationChainID, "resourceId", evt.ResourceID)

	// return msg.NewGenericTransfer(
	// 	l.chainId, // Unset
	// 	msg.ChainId(evt.DestinationChainID),
	// 	msg.Nonce(evt.DepositNonce),
	// 	msg.ResourceId(evt.ResourceID),
	// 	nil,
	// ), nil
}
