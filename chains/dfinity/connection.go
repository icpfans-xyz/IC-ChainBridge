package dfinity

import (
	"fmt"
	"sync"

	"github.com/ChainSafe/log15"
	"github.com/centrifuge/go-substrate-rpc-client/types"

	"github.com/icpfans-xyz/agent-go/agent"
	"github.com/icpfans-xyz/agent-go/agent/http"
	"github.com/icpfans-xyz/agent-go/agent/polling"
	"github.com/icpfans-xyz/agent-go/identity"
	"github.com/icpfans-xyz/agent-go/principal"
)

type Connection struct {
	agent          agent.Agent
	log            log15.Logger
	url            string           // API endpoint
	name           string           // Chain name
	key            identity.KeyPair // Keyring used for signing
	bridgeCanister string
	nonce          types.U32    // Latest account nonce
	nonceLock      sync.Mutex   // Locks nonce for updates
	stop           <-chan int   // Signals system shutdown, should be observed in all selects and loops
	sysErr         chan<- error // Propagates fatal errors to core
}

func NewConnection(url string, name string, bridgeCanister string, key []byte, log log15.Logger, stop <-chan int, sysErr chan<- error) *Connection {
	keyPair := identity.NewEd25519Identity(key)
	agent, _ := http.NewHttpAgent(http.HttpAgentOptions{
		Host:     url,
		Identity: agent.NewSignIdentity(keyPair, nil),
	})
	return &Connection{agent: agent, url: url, name: name, bridgeCanister: bridgeCanister, key: keyPair, log: log, stop: stop, sysErr: sysErr}
}

func (c *Connection) Relayer() string {
	return c.agent.GetPrincipal().ToString()
}

func (c *Connection) Connect() error {
	c.log.Info("Connecting to substrate chain...", "url", c.url)
	_, err := c.agent.Status()
	if err != nil {
		return err
	}

	return nil
}

func (c *Connection) SubmitTx(canisterId string, method string, args []byte) (interface{}, error) {
	c.log.Debug("Submitting substrate call...", "method", method, "sender", c.key.PublicKey())

	canister, err := principal.FromString(canisterId)
	if err != nil {
		return nil, err
	}
	options := agent.CallOptions{
		MethodName: method,
		Arg:        args,
	}

	resp, err := c.agent.Call(canister, &options)
	if err != nil {
		return nil, err
	}

	return c.watchSubmission(canister, resp.RequestId)
}

func (c *Connection) watchSubmission(canisterId *principal.Principal, requestID agent.RequestId) (interface{}, error) {
	// finalStatus := ""
	// var finalCert []byte
	// timer := time.NewTimer(timeout)
	// ticker := time.NewTicker(delay)
	// stopped := true
	// for stopped {
	// 	select {
	// 	case <-ticker.C:
	// 		paths := [][][]byte{{[]byte("request_status"), requestID[:]}}
	// 		resp, err := c.agent.ReadState(nil, &agent.ReadStateOptions{Paths: paths})
	// 		if err != nil {
	// 			fmt.Printf("can not request status raw with error : %v\n", err)
	// 		}
	// 		finalStatus = resp.StatusText
	// 		finalCert = resp.Certificate
	// 		if finalStatus == "replied" || finalStatus == "done" || finalStatus == "rejected" {
	// 			stopped = false
	// 		}
	// 	case <-timer.C:
	// 		stopped = false
	// 	}
	// }
	// if finalStatus == "replied" {
	// 	paths := [][]byte{[]byte("request_status"), requestID[:], []byte("reply")}
	// 	res, err := LookUp(paths, finalCert)
	// 	if err != nil {
	// 		return nil, err
	// 	}
	// 	return res, nil
	// }
	// defer timer.Stop()
	// defer ticker.Stop()
	// return nil, fmt.Errorf("call poll fail with status %v", finalStatus)
	return polling.PollForResponse(c.agent, canisterId, requestID, polling.DefaultStrategy())
}

func (c *Connection) Query(canisterId string, method string, args []byte, respObj interface{}) error {
	canister, err := principal.FromString(canisterId)
	if err != nil {
		return err
	}
	options := agent.QueryFields{
		MethodName: method,
		Arg:        args,
	}

	resp, err := c.agent.Query(canister, &options)
	if err != nil {
		return err
	}
	if resp.Status != agent.QueryResponseStatusReplied {
		return fmt.Errorf("query resp status reject:%s", resp.RejectMsg)
	}
	return nil
}
