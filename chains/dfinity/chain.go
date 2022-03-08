package dfinity

import (
	"github.com/ChainSafe/chainbridge-utils/blockstore"
	"github.com/ChainSafe/chainbridge-utils/core"
	"github.com/ChainSafe/chainbridge-utils/crypto/ed25519"
	"github.com/ChainSafe/chainbridge-utils/keystore"
	metrics "github.com/ChainSafe/chainbridge-utils/metrics/types"
	"github.com/ChainSafe/chainbridge-utils/msg"
	"github.com/ChainSafe/log15"
)

var _ core.Chain = &Chain{}

type Chain struct {
	cfg      *core.ChainConfig // The config of the chain
	conn     *Connection       // THe chains connection
	listener *listener         // The listener of this chain
	writer   *writer           // The writer of the chain
	stop     chan<- int
}

// checkBlockstore queries the blockstore for the latest known block. If the latest block is
// greater than startBlock, then the latest block is returned, otherwise startBlock is.
func checkBlockstore(bs *blockstore.Blockstore, startBlock uint64) (uint64, error) {
	latestBlock, err := bs.TryLoadLatestBlock()
	if err != nil {
		return 0, err
	}

	if latestBlock.Uint64() > startBlock {
		return latestBlock.Uint64(), nil
	} else {
		return startBlock, nil
	}
}

func InitializeChain(cfg *core.ChainConfig, logger log15.Logger, sysErr chan<- error, m *metrics.ChainMetrics) (*Chain, error) {
	kp, err := keystore.KeypairFromAddress(cfg.From, keystore.DftChain, cfg.KeystorePath, cfg.Insecure)
	if err != nil {
		return nil, err
	}

	krp := kp.(*ed25519.Keypair)

	// Attempt to load latest block
	bs, err := blockstore.NewBlockstore(cfg.BlockstorePath, cfg.Id, kp.Address())
	if err != nil {
		return nil, err
	}

	startIndex := parseStartIndex(cfg)
	if !cfg.FreshStart {
		startIndex, err = checkBlockstore(bs, startIndex)
		if err != nil {
			return nil, err
		}
	}

	bridgeCanister := parseBirdgeCanister(cfg)
	erc20Canister := parseERC20Canister(cfg)

	stop := make(chan int)
	// Setup connection
	conn := NewConnection(cfg.Endpoint, cfg.Name, bridgeCanister, krp.Encode(), logger, stop, sysErr)
	err = conn.Connect()
	if err != nil {
		return nil, err
	}

	ue := parseUseExtended(cfg)

	// Setup listener & writer

	l := NewListener(conn, cfg.Name, cfg.Id, startIndex, logger, bs, stop, sysErr, m)
	w := NewWriter(cfg.Id, conn, logger, erc20Canister, stop, sysErr, m, ue)
	return &Chain{
		cfg:      cfg,
		conn:     conn,
		listener: l,
		writer:   w,
		stop:     stop,
	}, nil
}

func (c *Chain) Start() error {
	err := c.listener.start()
	if err != nil {
		return err
	}
	c.conn.log.Debug("Successfully started chain", "chainId", c.cfg.Id)
	return nil
}

func (c *Chain) SetRouter(r *core.Router) {
	r.Listen(c.cfg.Id, c.writer)
	c.listener.setRouter(r)
}

func (c *Chain) LatestBlock() metrics.LatestBlock {
	return c.listener.latestBlock
}

func (c *Chain) Id() msg.ChainId {
	return c.cfg.Id
}

func (c *Chain) Name() string {
	return c.cfg.Name
}

func (c *Chain) Stop() {
	close(c.stop)
}
