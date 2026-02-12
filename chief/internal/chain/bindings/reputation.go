// Code generated - DO NOT EDIT.
// This file is a simplified Go binding for ERC-8004 IdentityRegistry and ReputationRegistry contracts.

package bindings

import (
	"math/big"
	"strings"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
)

// IdentityRegistryABI is the ABI of the ERC-8004 IdentityRegistry contract.
const IdentityRegistryABI = `[{"type":"function","name":"register","inputs":[{"name":"agentURI","type":"string"}],"outputs":[{"name":"agentId","type":"uint256"}],"stateMutability":"nonpayable"},{"type":"function","name":"ownerOf","inputs":[{"name":"agentId","type":"uint256"}],"outputs":[{"name":"","type":"address"}],"stateMutability":"view"},{"type":"function","name":"getAgentWallet","inputs":[{"name":"agentId","type":"uint256"}],"outputs":[{"name":"","type":"address"}],"stateMutability":"view"},{"type":"function","name":"setAgentWallet","inputs":[{"name":"agentId","type":"uint256"},{"name":"newWallet","type":"address"},{"name":"deadline","type":"uint256"},{"name":"signature","type":"bytes"}],"outputs":[],"stateMutability":"nonpayable"},{"type":"event","name":"Registered","inputs":[{"name":"agentId","type":"uint256","indexed":true},{"name":"agentURI","type":"string","indexed":false},{"name":"owner","type":"address","indexed":true}],"anonymous":false}]`

// IdentityRegistry is a Go binding for the ERC-8004 IdentityRegistry contract.
type IdentityRegistry struct {
	IdentityRegistryCaller
	IdentityRegistryTransactor
}

// IdentityRegistryCaller is the read-only binding.
type IdentityRegistryCaller struct {
	contract *bind.BoundContract
}

// IdentityRegistryTransactor is the write-only binding.
type IdentityRegistryTransactor struct {
	contract *bind.BoundContract
}

// NewIdentityRegistry creates a new instance of IdentityRegistry.
func NewIdentityRegistry(address common.Address, backend bind.ContractBackend) (*IdentityRegistry, error) {
	parsed, err := abi.JSON(strings.NewReader(IdentityRegistryABI))
	if err != nil {
		return nil, err
	}
	contract := bind.NewBoundContract(address, parsed, backend, backend, backend)
	return &IdentityRegistry{
		IdentityRegistryCaller:     IdentityRegistryCaller{contract: contract},
		IdentityRegistryTransactor: IdentityRegistryTransactor{contract: contract},
	}, nil
}

// OwnerOf returns the owner address for a given agent ID.
func (c *IdentityRegistryCaller) OwnerOf(opts *bind.CallOpts, agentId *big.Int) (common.Address, error) {
	var out []interface{}
	err := c.contract.Call(opts, &out, "ownerOf", agentId)
	if err != nil {
		return common.Address{}, err
	}
	return out[0].(common.Address), nil
}

// GetAgentWallet returns the wallet address for a given agent ID.
func (c *IdentityRegistryCaller) GetAgentWallet(opts *bind.CallOpts, agentId *big.Int) (common.Address, error) {
	var out []interface{}
	err := c.contract.Call(opts, &out, "getAgentWallet", agentId)
	if err != nil {
		return common.Address{}, err
	}
	return out[0].(common.Address), nil
}

// Register registers the caller as an agent with the given URI.
func (t *IdentityRegistryTransactor) Register(opts *bind.TransactOpts, agentURI string) (*types.Transaction, error) {
	return t.contract.Transact(opts, "register", agentURI)
}

// ReputationRegistryABI is the ABI of the ERC-8004 ReputationRegistry contract.
const ReputationRegistryABI = `[{"type":"function","name":"giveFeedback","inputs":[{"name":"agentId","type":"uint256"},{"name":"value","type":"int128"},{"name":"valueDecimals","type":"uint8"},{"name":"tag1","type":"string"},{"name":"tag2","type":"string"},{"name":"endpoint","type":"string"},{"name":"feedbackURI","type":"string"},{"name":"feedbackHash","type":"bytes32"}],"outputs":[],"stateMutability":"nonpayable"},{"type":"function","name":"getSummary","inputs":[{"name":"agentId","type":"uint256"},{"name":"clientAddresses","type":"address[]"},{"name":"tag1","type":"string"},{"name":"tag2","type":"string"}],"outputs":[{"name":"count","type":"uint64"},{"name":"summaryValue","type":"int128"},{"name":"summaryValueDecimals","type":"uint8"}],"stateMutability":"view"},{"type":"event","name":"NewFeedback","inputs":[{"name":"agentId","type":"uint256","indexed":true},{"name":"clientAddress","type":"address","indexed":true},{"name":"feedbackIndex","type":"uint64","indexed":false},{"name":"value","type":"int128","indexed":false},{"name":"valueDecimals","type":"uint8","indexed":false},{"name":"indexedTag1","type":"string","indexed":true},{"name":"tag1","type":"string","indexed":false},{"name":"tag2","type":"string","indexed":false},{"name":"endpoint","type":"string","indexed":false},{"name":"feedbackURI","type":"string","indexed":false},{"name":"feedbackHash","type":"bytes32","indexed":false}],"anonymous":false}]`

// ReputationData holds reputation query results.
type ReputationData struct {
	Count                uint64
	SummaryValue         *big.Int
	SummaryValueDecimals uint8
}

// ReputationRegistry is a Go binding for the ERC-8004 ReputationRegistry contract.
type ReputationRegistry struct {
	ReputationRegistryCaller
	ReputationRegistryTransactor
}

// ReputationRegistryCaller is the read-only binding.
type ReputationRegistryCaller struct {
	contract *bind.BoundContract
}

// ReputationRegistryTransactor is the write-only binding.
type ReputationRegistryTransactor struct {
	contract *bind.BoundContract
}

// NewReputationRegistry creates a new instance of ReputationRegistry.
func NewReputationRegistry(address common.Address, backend bind.ContractBackend) (*ReputationRegistry, error) {
	parsed, err := abi.JSON(strings.NewReader(ReputationRegistryABI))
	if err != nil {
		return nil, err
	}
	contract := bind.NewBoundContract(address, parsed, backend, backend, backend)
	return &ReputationRegistry{
		ReputationRegistryCaller:     ReputationRegistryCaller{contract: contract},
		ReputationRegistryTransactor: ReputationRegistryTransactor{contract: contract},
	}, nil
}

// GetSummary returns the reputation summary for an agent.
func (c *ReputationRegistryCaller) GetSummary(opts *bind.CallOpts, agentId *big.Int, clientAddresses []common.Address, tag1, tag2 string) (uint64, *big.Int, uint8, error) {
	var out []interface{}
	err := c.contract.Call(opts, &out, "getSummary", agentId, clientAddresses, tag1, tag2)
	if err != nil {
		return 0, nil, 0, err
	}
	return out[0].(uint64), out[1].(*big.Int), out[2].(uint8), nil
}

// GiveFeedback submits reputation feedback for an agent.
func (t *ReputationRegistryTransactor) GiveFeedback(opts *bind.TransactOpts, agentId *big.Int, value *big.Int, valueDecimals uint8, tag1, tag2, endpoint, feedbackURI string, feedbackHash [32]byte) (*types.Transaction, error) {
	return t.contract.Transact(opts, "giveFeedback", agentId, value, valueDecimals, tag1, tag2, endpoint, feedbackURI, feedbackHash)
}
