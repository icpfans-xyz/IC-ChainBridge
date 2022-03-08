package dfinity

import (
	"errors"
	"fmt"
	"math/big"
	"time"

	"github.com/ChainSafe/ChainBridge/chains"
	"github.com/ChainSafe/chainbridge-utils/blockstore"
	metrics "github.com/ChainSafe/chainbridge-utils/metrics/types"
	"github.com/ChainSafe/chainbridge-utils/msg"
	"github.com/ChainSafe/log15"
	"github.com/icpfans-xyz/agent-go/candid/idl"
)

type listener struct {
	name        string
	chainId     msg.ChainId
	startIndex  uint64
	blockstore  blockstore.Blockstorer
	conn        *Connection
	router      chains.Router
	log         log15.Logger
	stop        <-chan int
	sysErr      chan<- error
	latestBlock metrics.LatestBlock
	metrics     *metrics.ChainMetrics
}

// Frequency of polling for a new block
var BlockRetryInterval = time.Second * 5
var BlockRetryLimit = 5

func NewListener(conn *Connection, name string, id msg.ChainId, startIndex uint64, log log15.Logger, bs blockstore.Blockstorer, stop <-chan int, sysErr chan<- error, m *metrics.ChainMetrics) *listener {
	return &listener{
		name:        name,
		chainId:     id,
		startIndex:  startIndex,
		blockstore:  bs,
		conn:        conn,
		log:         log,
		stop:        stop,
		sysErr:      sysErr,
		latestBlock: metrics.LatestBlock{LastUpdated: time.Now()},
		metrics:     m,
	}
}

func (l *listener) setRouter(r chains.Router) {
	l.router = r
}

// start creates the initial subscription for all events
func (l *listener) start() error {
	// Check whether latest is less than starting block
	startIndex, err := l.getLatestIndex()
	if err != nil {
		return err
	}
	if startIndex < l.startIndex {
		return fmt.Errorf("starting Index (%d) is greater than latest known Index (%d)", l.startIndex, startIndex)
	}

	go func() {
		err := l.pollBlocks()
		if err != nil {
			l.log.Error("Polling blocks failed", "err", err)
		}
	}()

	return nil
}

func (l *listener) getLatestIndex() (uint64, error) {
	var index uint
	err := l.conn.Query(l.conn.bridgeCanister, MethodDepositRecordLastIndex, nil, &index)
	if err != nil {
		return 0, err
	}
	return uint64(index), nil
}

func (l *listener) getDepositRecordByIndex(index uint64) (*DepositRecord, error) {
	params, err := idl.Encode([]idl.Type{new(idl.Nat)}, []interface{}{big.NewInt(int64(index))})
	if err != nil {
		return nil, err
	}
	var record DepositRecord
	err = l.conn.Query(l.conn.bridgeCanister, MethodDepositRecord, params, &record)
	if err != nil {
		return nil, err
	}
	return &record, nil
}

var ErrBlockNotReady = errors.New("required result to be 32 bytes, but got 0")

// pollBlocks will poll for the latest block and proceed to parse the associated events as it sees new blocks.
// Polling begins at the block defined in `l.startBlock`. Failed attempts to fetch the latest block or parse
// a block will be retried up to BlockRetryLimit times before returning with an error.
func (l *listener) pollBlocks() error {
	currentBlock := l.startIndex
	var retry = BlockRetryLimit
	for {
		select {
		case <-l.stop:
			return errors.New("terminated")
		default:
			// No more retries, goto next block
			if retry == 0 {
				l.sysErr <- fmt.Errorf("event polling retries exceeded (chain=%d, name=%s)", l.chainId, l.name)
				return nil
			}

			// Get hash for latest block, sleep and retry if not ready
			lastIndex, err := l.getLatestIndex()
			if err != nil && err.Error() == ErrBlockNotReady.Error() {
				time.Sleep(BlockRetryInterval)
				continue
			} else if err != nil {
				l.log.Error("Failed to query latest block", "block", currentBlock, "err", err)
				retry--
				time.Sleep(BlockRetryInterval)
				continue
			}

			err = l.processEvents(lastIndex)
			if err != nil {
				l.log.Error("Failed to process events in block", "block", currentBlock, "err", err)
				retry--
				continue
			}

			if l.metrics != nil {
				l.metrics.BlocksProcessed.Inc()
				l.metrics.LatestProcessedBlock.Set(float64(currentBlock))
			}

			// currentBlock++
			l.latestBlock.Height = big.NewInt(0).SetUint64(currentBlock)
			l.latestBlock.LastUpdated = time.Now()
			retry = BlockRetryLimit
		}
	}
}

// processEvents fetches a block and parses out the events, calling Listener.handleEvents()
func (l *listener) processEvents(end uint64) error {
	l.log.Trace("Fetching block for events", "start", l.startIndex, "end", end)

	index := l.startIndex
	for index < end {
		record, err := l.getDepositRecordByIndex(index)
		if err != nil {
			return err
		}
		if err := l.handleRecord(record); err != nil {
			return err
		}
		// Write to blockstore
		l.startIndex = index
		err = l.blockstore.StoreBlock(big.NewInt(0).SetUint64(index))
		if err != nil {
			return err
			l.log.Error("Failed to write to blockstore", "err", err)
		}
		index++
	}
	l.log.Trace("Finished processing events", "end", end)

	return nil
}

// handleEvents calls the associated handler for all registered event types
func (l *listener) handleRecord(record *DepositRecord) error {
	l.log.Trace("Handling FungibleTransfer event")
	msg, err := l.HandleFungibleTransfer(record)
	if err != nil {
		return err
	}
	return l.submitMessage(msg)
}

// submitMessage inserts the chainId into the msg and sends it to the router
func (l *listener) submitMessage(m msg.Message) error {
	err := l.router.Send(m)
	if err != nil {
		log15.Error("failed to process event", "err", err)
		return err
	}
	return nil
}
