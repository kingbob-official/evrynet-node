package staking

import (
	"context"
	"math/big"
	"sort"
	"strings"

	"github.com/pkg/errors"

	ethereum "github.com/Evrynetlabs/evrynet-node"
	"github.com/Evrynetlabs/evrynet-node/common"
	"github.com/Evrynetlabs/evrynet-node/common/math"
	"github.com/Evrynetlabs/evrynet-node/consensus"
	"github.com/Evrynetlabs/evrynet-node/consensus/staking_contracts"
	"github.com/Evrynetlabs/evrynet-node/core"
	"github.com/Evrynetlabs/evrynet-node/core/state"
	"github.com/Evrynetlabs/evrynet-node/core/types"
	"github.com/Evrynetlabs/evrynet-node/core/vm"
	"github.com/Evrynetlabs/evrynet-node/params"
)

var (
	// ErrEmptyValidatorSet returns when valset from smart-contract is empty
	ErrEmptyValidatorSet = errors.New("empty validator set")
	// ErrLengthOfCandidatesAndStakesMisMatch returns when lengths stakes and candidates are not match
	ErrLengthOfCandidatesAndStakesMisMatch = errors.New("length of stakes is not equal to length of candidates")
	indexValidatorMapping                  = map[string]uint64{
		"validators": 0,
	}

	maxGasGetValSet uint64 = 500000000
)

type StakingCaller interface {
	GetValidators(common.Address) ([]common.Address, error)
}

// BackendContractCaller creates a wrapper with statedb to implements ContractCaller
type BackendContractCaller struct {
	blockNumber  *big.Int
	header       *types.Header
	stateDB      *state.StateDB
	chainContext core.ChainContext
	chainConfig  *params.ChainConfig
	vmConfig     vm.Config
}

// GetValidators returns validators from stateDB and block number of the caller by smart-contract's address
func (caller *BackendContractCaller) GetValidators(scAddress common.Address) ([]common.Address, error) {
	sc, err := staking_contracts.NewStakingContractsCaller(scAddress, caller)
	if err != nil {
		return nil, err
	}
	data, err := sc.GetListCandidatesWithCurrentData(nil)
	if err != nil {
		return nil, err
	}
	// sanity checks
	if len(data.Candidates) != len(data.Stakes) {
		return nil, ErrLengthOfCandidatesAndStakesMisMatch
	}

	if len(data.Candidates) == 0 {
		return nil, ErrEmptyValidatorSet
	}

	if len(data.Candidates) < int(data.MaxValSize) {
		return data.Candidates, nil
	}
	// sort and returns topN
	stakes := make(map[common.Address]*big.Int)
	for i := 0; i < len(data.Candidates); i++ {
		stakes[data.Candidates[i]] = data.Stakes[i]
	}
	sort.Slice(data.Candidates, func(i, j int) bool {
		if stakes[data.Candidates[i]].Cmp(stakes[data.Candidates[j]]) == 0 {
			return strings.Compare(data.Candidates[i].String(), data.Candidates[j].String()) > 0
		}
		return stakes[data.Candidates[i]].Cmp(stakes[data.Candidates[j]]) > 0
	})
	return data.Candidates[:int(data.MaxValSize)], err
}

// NewBECaller returns
func NewStakingCaller(stateDB *state.StateDB, chainContext core.ChainContext, header *types.Header,
	chainConfig *params.ChainConfig, vmConfig vm.Config) StakingCaller {
	return &BackendContractCaller{
		stateDB:      stateDB,
		chainContext: chainContext,
		blockNumber:  header.Number,
		header:       header,
		chainConfig:  chainConfig,
		vmConfig:     vmConfig,
	}
}

// CodeAt returns the code of the given account. This is needed to differentiate
// between contract internal errors and the local chain being out of sync.
func (caller *BackendContractCaller) CodeAt(ctx context.Context, contract common.Address, blockNumber *big.Int) ([]byte, error) {
	return caller.stateDB.GetCode(contract), nil
}

// ContractCall executes an Evrynet contract call with the specified data as the
// input.
func (caller *BackendContractCaller) CallContract(ctx context.Context, call ethereum.CallMsg, blockNumber *big.Int) ([]byte, error) {
	clonedStateDB := caller.stateDB.Copy()
	if blockNumber != nil && blockNumber.Cmp(caller.blockNumber) != 0 {
		return nil, errors.New("blockNumber is not supported")
	}
	if call.GasPrice == nil {
		call.GasPrice = big.NewInt(1)
	}
	if call.Gas == 0 {
		call.Gas = maxGasGetValSet
	}
	if call.Value == nil {
		call.Value = new(big.Int)
	}
	from := clonedStateDB.GetOrNewStateObject(call.From)
	from.SetBalance(math.MaxBig256)
	// Execute the call.
	msg := callmsg{call}
	evmContext := core.NewEVMContext(msg, caller.header, caller.chainContext, nil)
	// Create a new environment which holds all relevant information
	// about the transaction and calling mechanisms.
	vmenv := vm.NewEVM(evmContext, clonedStateDB, caller.chainConfig, caller.vmConfig)
	defer vmenv.Cancel()
	gaspool := new(core.GasPool).AddGas(maxGasGetValSet)
	rval, _, _, err := core.NewStateTransition(vmenv, msg, gaspool).TransitionDb()
	return rval, err
}

// callmsg implements core.Message to allow passing it as a transaction simulator.
type callmsg struct {
	ethereum.CallMsg
}

func (m callmsg) GasPayer() common.Address  { return m.CallMsg.From }
func (m callmsg) Owner() *common.Address    { return nil }
func (m callmsg) Provider() *common.Address { return nil }
func (m callmsg) From() common.Address      { return m.CallMsg.From }
func (m callmsg) Nonce() uint64             { return 0 }
func (m callmsg) CheckNonce() bool          { return false }
func (m callmsg) To() *common.Address       { return m.CallMsg.To }
func (m callmsg) GasPrice() *big.Int        { return m.CallMsg.GasPrice }
func (m callmsg) Gas() uint64               { return m.CallMsg.Gas }
func (m callmsg) Value() *big.Int           { return m.CallMsg.Value }
func (m callmsg) Data() []byte              { return m.CallMsg.Data }

type chainContextWrapper struct {
	engine      consensus.Engine
	getHeaderFn func(common.Hash, uint64) *types.Header
}

func (w *chainContextWrapper) Engine() consensus.Engine {
	return w.engine
}

func (w *chainContextWrapper) GetHeader(hash common.Hash, height uint64) *types.Header {
	return w.getHeaderFn(hash, height)
}

func NewChainContextWrapper(engine consensus.Engine, getHeaderFn func(common.Hash, uint64) *types.Header) core.ChainContext {
	return &chainContextWrapper{
		engine:      engine,
		getHeaderFn: getHeaderFn,
	}
}
