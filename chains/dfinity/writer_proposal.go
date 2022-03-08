package dfinity

import (
	"errors"
	"math/big"
	"time"

	"github.com/ChainSafe/chainbridge-utils/msg"
	"github.com/icpfans-xyz/agent-go/candid/idl"
	"github.com/prometheus/common/log"
)

// Number of blocks to wait for an finalization event
const ExecuteBlockWatchLimit = 100

// Time between retrying a failed tx
const TxRetryInterval = time.Second * 2

// Maximum number of tx retries before exiting
const TxRetryLimit = 10

var ErrNonceTooLow = errors.New("nonce too low")
var ErrTxUnderpriced = errors.New("replacement transaction underpriced")
var ErrFatalTx = errors.New("submission of transaction failed")
var ErrFatalQuery = errors.New("query of chain state failed")

func (w *writer) createFungibleProposal(m msg.Message) bool {
	w.log.Info("Creating erc20 proposal", "src", m.Source, "nonce", m.DepositNonce)

	dataHash := w.calculateDataHash(m)

	if !w.shouldVote(m, dataHash) {
		if w.proposalIsPassed(m.Source, m.DepositNonce, dataHash) {
			// We should not vote for this proposal but it is ready to be executed
			w.executeProposal(m, dataHash)
			return true
		} else {
			return false
		}
	}

	// watch for execution event
	go w.watchThenExecute(m, dataHash)

	w.voteProposal(m, dataHash)

	return true
}

func (w *writer) createNonFungibleProposal(m msg.Message) bool {
	w.log.Error("Nonfungible proposal not support")
	return false
}

func (w *writer) createGenericProposal(m msg.Message) bool {
	w.log.Error("generic proposal not support")
	return false
}

func (w *writer) GetProposal(srcId msg.ChainId, nonce msg.Nonce, dataHash string) (*Proposal, error) {
	pTypes := []idl.Type{idl.Nat16(), idl.Nat64(), new(idl.Text)}
	pValues := []interface{}{big.NewInt(int64(srcId)), big.NewInt(int64(nonce)), dataHash}
	params, err := idl.Encode(pTypes, pValues)
	if err != nil {
		w.log.Error("failed to encode", "src", srcId, "nonce", nonce, "dataHash", dataHash)
		return nil, err
	}
	var prop Proposal
	err = w.conn.Query(w.conn.bridgeCanister, MethodGetProposal, params, &prop)
	if err != nil {
		w.log.Error("Failed to check proposal existence", "err", err)
		return nil, err
	}
	return &prop, nil
}

// proposalIsComplete returns true if the proposal state is either Passed, Transferred or Cancelled
func (w *writer) proposalIsComplete(srcId msg.ChainId, nonce msg.Nonce, dataHash string) bool {
	prop, err := w.GetProposal(srcId, nonce, dataHash)
	if err != nil {
		w.log.Error("Failed to check proposal existence", "err", err)
		return false
	}
	return prop.Status == PassedStatus || prop.Status == TransferredStatus || prop.Status == CancelledStatus
}

// proposalIsComplete returns true if the proposal state is Transferred or Cancelled
func (w *writer) proposalIsFinalized(srcId msg.ChainId, nonce msg.Nonce, dataHash string) bool {
	prop, err := w.GetProposal(srcId, nonce, dataHash)
	if err != nil {
		w.log.Error("Failed to check proposal existence", "err", err)
		return false
	}
	return prop.Status == TransferredStatus || prop.Status == CancelledStatus // Transferred (3)
}

func (w *writer) proposalIsPassed(srcId msg.ChainId, nonce msg.Nonce, dataHash string) bool {
	prop, err := w.GetProposal(srcId, nonce, dataHash)
	if err != nil {
		w.log.Error("Failed to check proposal existence", "err", err)
		return false
	}
	return prop.Status == PassedStatus
}

// hasVoted checks if this relayer has already voted
func (w *writer) hasVoted(srcId msg.ChainId, nonce msg.Nonce, dataHash string) bool {
	pTypes := []idl.Type{idl.Nat16(), idl.Nat64(), new(idl.Text), new(idl.Text)}
	pValues := []interface{}{big.NewInt(int64(srcId)), big.NewInt(int64(nonce)), dataHash, w.conn.Relayer()}
	params, err := idl.Encode(pTypes, pValues)
	if err != nil {
		w.log.Error("failed to encode", "src", srcId, "nonce", nonce, "dataHash", dataHash)
		return false
	}
	var hasVoted bool
	err = w.conn.Query(w.conn.bridgeCanister, MethodHasVotedProposal, params, &hasVoted)
	if err != nil {
		w.log.Error("Failed to check proposal existence", "err", err)
		return false
	}

	return hasVoted
}

func (w *writer) shouldVote(m msg.Message, dataHash string) bool {
	// Check if proposal has passed and skip if Passed or Transferred
	if w.proposalIsComplete(m.Source, m.DepositNonce, dataHash) {
		w.log.Info("Proposal complete, not voting", "src", m.Source, "nonce", m.DepositNonce)
		return false
	}

	// Check if relayer has previously voted
	if w.hasVoted(m.Source, m.DepositNonce, dataHash) {
		w.log.Info("Relayer has already voted, not voting", "src", m.Source, "nonce", m.DepositNonce)
		return false
	}

	return true
}

// watchThenExecute watches for the latest block and executes once the matching finalized event is found
func (w *writer) watchThenExecute(m msg.Message, dataHash string) {
	w.log.Info("Watching for finalization event", "src", m.Source, "nonce", m.DepositNonce)

	// watching for the latest block, querying and matching the finalized event will be retried up to ExecuteBlockWatchLimit times
	for i := 0; i < ExecuteBlockWatchLimit; i++ {
		select {
		case <-w.stop:
			return
		default:
			time.Sleep(TxRetryInterval)

			prop, err := w.GetProposal(m.Source, m.DepositNonce, dataHash)
			if err != nil {
				log.Error("failed to get proposal", err)
			}
			if prop.Status == TransferredStatus {
				w.executeProposal(m, dataHash)
				return
			} else if prop.Status == TransferredStatus || prop.Status == CancelledStatus {
				return
			}
		}
	}
	log.Warn("Block watch limit exceeded, skipping execution", "source", m.Source, "dest", m.Destination, "nonce", m.DepositNonce)
}

// voteProposal submits a vote proposal
// a vote proposal will try to be submitted up to the TxRetryLimit times
func (w *writer) voteProposal(m msg.Message, dataHash string) {
	for i := 0; i < TxRetryLimit; i++ {
		select {
		case <-w.stop:
			return
		default:

			pTypes := []idl.Type{idl.Nat16(), idl.Nat64(), new(idl.Text), new(idl.Text)}
			pValues := []interface{}{big.NewInt(int64(w.chainId)), big.NewInt(int64(m.DepositNonce)), m.ResourceId.Hex(), dataHash}
			params, err := idl.Encode(pTypes, pValues)
			if err != nil {
				w.log.Error("failed to encode", "msg", m)
				break
			}
			tx, err := w.conn.SubmitTx(w.conn.bridgeCanister, MethodVoteProposal, params)

			if err == nil {
				w.log.Info("Submitted proposal vote", "tx", tx, "src", m.Source, "depositNonce", m.DepositNonce)
				if w.metrics != nil {
					w.metrics.VotesSubmitted.Inc()
				}
				return
			} else {
				w.log.Warn("Voting failed", "source", m.Source, "dest", m.Destination, "depositNonce", m.DepositNonce, "err", err)
				time.Sleep(TxRetryInterval)
			}

			// Verify proposal is still open for voting, otherwise no need to retry
			if w.proposalIsComplete(m.Source, m.DepositNonce, dataHash) {
				w.log.Info("Proposal voting complete on chain", "src", m.Source, "dst", m.Destination, "nonce", m.DepositNonce)
				return
			}
		}
	}
	w.log.Error("Submission of Vote transaction failed", "source", m.Source, "dest", m.Destination, "depositNonce", m.DepositNonce)
	w.sysErr <- ErrFatalTx
}

// executeProposal executes the proposal
func (w *writer) executeProposal(m msg.Message, dataHash string) {
	for i := 0; i < TxRetryLimit; i++ {
		select {
		case <-w.stop:
			return
		default:
			amount := big.NewInt(0).SetBytes(m.Payload[0].([]byte))
			recipient := string(m.Payload[1].([]byte))
			pTypes := []idl.Type{idl.Nat16(), idl.Nat64(), new(idl.Text), new(idl.Text), new(idl.Nat)}
			pValues := []interface{}{big.NewInt(int64(w.chainId)), big.NewInt(int64(m.DepositNonce)), m.ResourceId.Hex(), recipient, amount}
			params, err := idl.Encode(pTypes, pValues)
			if err != nil {
				w.log.Error("failed to encode", "msg", m)
				break
			}
			tx, err := w.conn.SubmitTx(w.conn.bridgeCanister, MethodExecuteProposal, params)

			if err == nil {
				w.log.Info("Submitted proposal execution", "tx", tx, "src", m.Source, "dst", m.Destination, "nonce", m.DepositNonce)
				return
			} else {
				w.log.Warn("Execution failed, proposal may already be complete", "err", err)
				time.Sleep(TxRetryInterval)
			}

			// Verify proposal is still open for execution, tx will fail if we aren't the first to execute,
			// but there is no need to retry
			if w.proposalIsFinalized(m.Source, m.DepositNonce, dataHash) {
				w.log.Info("Proposal finalized on chain", "src", m.Source, "dst", m.Destination, "nonce", m.DepositNonce)
				return
			}
		}
	}
	w.log.Error("Submission of Execute transaction failed", "source", m.Source, "dest", m.Destination, "depositNonce", m.DepositNonce)
	w.sysErr <- ErrFatalTx
}
