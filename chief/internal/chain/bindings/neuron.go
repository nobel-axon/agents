// Code generated - DO NOT EDIT.
// This file is a simplified Go binding for NeuronToken (ERC20) contract.

package bindings

import (
	"math/big"
	"strings"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
)

// NeuronTokenABI is the ABI of the NeuronToken contract.
const NeuronTokenABI = `[{"type":"function","name":"DOMAIN_SEPARATOR","inputs":[],"outputs":[{"name":"","type":"bytes32","internalType":"bytes32"}],"stateMutability":"view"},{"type":"function","name":"allowance","inputs":[{"name":"owner","type":"address","internalType":"address"},{"name":"spender","type":"address","internalType":"address"}],"outputs":[{"name":"","type":"uint256","internalType":"uint256"}],"stateMutability":"view"},{"type":"function","name":"approve","inputs":[{"name":"spender","type":"address","internalType":"address"},{"name":"amount","type":"uint256","internalType":"uint256"}],"outputs":[{"name":"","type":"bool","internalType":"bool"}],"stateMutability":"nonpayable"},{"type":"function","name":"balanceOf","inputs":[{"name":"account","type":"address","internalType":"address"}],"outputs":[{"name":"","type":"uint256","internalType":"uint256"}],"stateMutability":"view"},{"type":"function","name":"burn","inputs":[{"name":"amount","type":"uint256","internalType":"uint256"}],"outputs":[],"stateMutability":"nonpayable"},{"type":"function","name":"burnFrom","inputs":[{"name":"account","type":"address","internalType":"address"},{"name":"amount","type":"uint256","internalType":"uint256"}],"outputs":[],"stateMutability":"nonpayable"},{"type":"function","name":"decimals","inputs":[],"outputs":[{"name":"","type":"uint8","internalType":"uint8"}],"stateMutability":"view"},{"type":"function","name":"name","inputs":[],"outputs":[{"name":"","type":"string","internalType":"string"}],"stateMutability":"view"},{"type":"function","name":"nonces","inputs":[{"name":"owner","type":"address","internalType":"address"}],"outputs":[{"name":"","type":"uint256","internalType":"uint256"}],"stateMutability":"view"},{"type":"function","name":"permit","inputs":[{"name":"owner","type":"address","internalType":"address"},{"name":"spender","type":"address","internalType":"address"},{"name":"value","type":"uint256","internalType":"uint256"},{"name":"deadline","type":"uint256","internalType":"uint256"},{"name":"v","type":"uint8","internalType":"uint8"},{"name":"r","type":"bytes32","internalType":"bytes32"},{"name":"s","type":"bytes32","internalType":"bytes32"}],"outputs":[],"stateMutability":"nonpayable"},{"type":"function","name":"symbol","inputs":[],"outputs":[{"name":"","type":"string","internalType":"string"}],"stateMutability":"view"},{"type":"function","name":"totalSupply","inputs":[],"outputs":[{"name":"","type":"uint256","internalType":"uint256"}],"stateMutability":"view"},{"type":"function","name":"transfer","inputs":[{"name":"to","type":"address","internalType":"address"},{"name":"amount","type":"uint256","internalType":"uint256"}],"outputs":[{"name":"","type":"bool","internalType":"bool"}],"stateMutability":"nonpayable"},{"type":"function","name":"transferFrom","inputs":[{"name":"from","type":"address","internalType":"address"},{"name":"to","type":"address","internalType":"address"},{"name":"amount","type":"uint256","internalType":"uint256"}],"outputs":[{"name":"","type":"bool","internalType":"bool"}],"stateMutability":"nonpayable"},{"type":"event","name":"Approval","inputs":[{"name":"owner","type":"address","indexed":true,"internalType":"address"},{"name":"spender","type":"address","indexed":true,"internalType":"address"},{"name":"value","type":"uint256","indexed":false,"internalType":"uint256"}],"anonymous":false},{"type":"event","name":"Transfer","inputs":[{"name":"from","type":"address","indexed":true,"internalType":"address"},{"name":"to","type":"address","indexed":true,"internalType":"address"},{"name":"value","type":"uint256","indexed":false,"internalType":"uint256"}],"anonymous":false}]`

// NeuronToken is a Go binding for the NeuronToken contract.
type NeuronToken struct {
	NeuronTokenCaller
	NeuronTokenTransactor
}

// NeuronTokenCaller is the read-only binding to the contract.
type NeuronTokenCaller struct {
	contract *bind.BoundContract
}

// NeuronTokenTransactor is the write-only binding to the contract.
type NeuronTokenTransactor struct {
	contract *bind.BoundContract
}

// NewNeuronToken creates a new instance of NeuronToken.
func NewNeuronToken(address common.Address, backend bind.ContractBackend) (*NeuronToken, error) {
	parsed, err := abi.JSON(strings.NewReader(NeuronTokenABI))
	if err != nil {
		return nil, err
	}
	contract := bind.NewBoundContract(address, parsed, backend, backend, backend)
	return &NeuronToken{
		NeuronTokenCaller:     NeuronTokenCaller{contract: contract},
		NeuronTokenTransactor: NeuronTokenTransactor{contract: contract},
	}, nil
}

// BalanceOf returns the balance of an address.
func (c *NeuronTokenCaller) BalanceOf(opts *bind.CallOpts, account common.Address) (*big.Int, error) {
	var out []interface{}
	err := c.contract.Call(opts, &out, "balanceOf", account)
	if err != nil {
		return nil, err
	}
	return out[0].(*big.Int), nil
}

// TotalSupply returns the total supply.
func (c *NeuronTokenCaller) TotalSupply(opts *bind.CallOpts) (*big.Int, error) {
	var out []interface{}
	err := c.contract.Call(opts, &out, "totalSupply")
	if err != nil {
		return nil, err
	}
	return out[0].(*big.Int), nil
}

// Decimals returns the decimals.
func (c *NeuronTokenCaller) Decimals(opts *bind.CallOpts) (uint8, error) {
	var out []interface{}
	err := c.contract.Call(opts, &out, "decimals")
	if err != nil {
		return 0, err
	}
	return out[0].(uint8), nil
}

// Name returns the name.
func (c *NeuronTokenCaller) Name(opts *bind.CallOpts) (string, error) {
	var out []interface{}
	err := c.contract.Call(opts, &out, "name")
	if err != nil {
		return "", err
	}
	return out[0].(string), nil
}

// Symbol returns the symbol.
func (c *NeuronTokenCaller) Symbol(opts *bind.CallOpts) (string, error) {
	var out []interface{}
	err := c.contract.Call(opts, &out, "symbol")
	if err != nil {
		return "", err
	}
	return out[0].(string), nil
}

// Allowance returns the allowance.
func (c *NeuronTokenCaller) Allowance(opts *bind.CallOpts, owner, spender common.Address) (*big.Int, error) {
	var out []interface{}
	err := c.contract.Call(opts, &out, "allowance", owner, spender)
	if err != nil {
		return nil, err
	}
	return out[0].(*big.Int), nil
}

// Transfer transfers tokens.
func (t *NeuronTokenTransactor) Transfer(opts *bind.TransactOpts, to common.Address, amount *big.Int) (*types.Transaction, error) {
	return t.contract.Transact(opts, "transfer", to, amount)
}

// Approve approves an allowance.
func (t *NeuronTokenTransactor) Approve(opts *bind.TransactOpts, spender common.Address, amount *big.Int) (*types.Transaction, error) {
	return t.contract.Transact(opts, "approve", spender, amount)
}

// Burn burns tokens.
func (t *NeuronTokenTransactor) Burn(opts *bind.TransactOpts, amount *big.Int) (*types.Transaction, error) {
	return t.contract.Transact(opts, "burn", amount)
}

// BurnFrom burns tokens from another address.
func (t *NeuronTokenTransactor) BurnFrom(opts *bind.TransactOpts, account common.Address, amount *big.Int) (*types.Transaction, error) {
	return t.contract.Transact(opts, "burnFrom", account, amount)
}
