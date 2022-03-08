package dfinity

import (
	"strconv"

	"github.com/ChainSafe/chainbridge-utils/core"
)

func parseBirdgeCanister(cfg *core.ChainConfig) string {
	if b, ok := cfg.Opts["bridgeCanister"]; ok {
		return b
	}
	panic("bridge canister not config")
	return ""
}

func parseERC20Canister(cfg *core.ChainConfig) string {
	if b, ok := cfg.Opts["erc20Canister"]; ok {
		return b
	}
	panic("erc20 canister not config")
	return ""
}

func parseStartIndex(cfg *core.ChainConfig) uint64 {
	if blk, ok := cfg.Opts["startIndex"]; ok {
		res, err := strconv.ParseUint(blk, 10, 32)
		if err != nil {
			panic(err)
		}
		return res
	}
	return 0
}

func parseUseExtended(cfg *core.ChainConfig) bool {
	if b, ok := cfg.Opts["useExtendedCall"]; ok {
		res, err := strconv.ParseBool(b)
		if err != nil {
			panic(err)
		}
		return res
	}
	return false
}
