// Package bindings provides Go bindings for nad.fun contracts.
package bindings

import (
	"math/big"
	"strings"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
)

// NadFunLensABI is the ABI for the nad.fun Lens contract.
// Lens provides unified query interface for both bonding curve and DEX phases.
const NadFunLensABI = `[
	{
		"type": "function",
		"name": "getAmountOut",
		"inputs": [
			{"name": "token", "type": "address", "internalType": "address"},
			{"name": "amountIn", "type": "uint256", "internalType": "uint256"},
			{"name": "isBuy", "type": "bool", "internalType": "bool"}
		],
		"outputs": [
			{"name": "router", "type": "address", "internalType": "address"},
			{"name": "amountOut", "type": "uint256", "internalType": "uint256"}
		],
		"stateMutability": "view"
	},
	{
		"type": "function",
		"name": "isGraduated",
		"inputs": [
			{"name": "token", "type": "address", "internalType": "address"}
		],
		"outputs": [
			{"name": "", "type": "bool", "internalType": "bool"}
		],
		"stateMutability": "view"
	},
	{
		"type": "function",
		"name": "isLocked",
		"inputs": [
			{"name": "token", "type": "address", "internalType": "address"}
		],
		"outputs": [
			{"name": "", "type": "bool", "internalType": "bool"}
		],
		"stateMutability": "view"
	},
	{
		"type": "function",
		"name": "getProgress",
		"inputs": [
			{"name": "token", "type": "address", "internalType": "address"}
		],
		"outputs": [
			{"name": "", "type": "uint256", "internalType": "uint256"}
		],
		"stateMutability": "view"
	}
]`

// NadFunBondingCurveRouterABI is the ABI for the nad.fun BondingCurveRouter.
// Used when token is in bonding curve phase (not yet graduated).
const NadFunBondingCurveRouterABI = `[
	{
		"type": "function",
		"name": "buy",
		"inputs": [
			{
				"name": "params",
				"type": "tuple",
				"internalType": "struct BuyParams",
				"components": [
					{"name": "amountOutMin", "type": "uint256", "internalType": "uint256"},
					{"name": "token", "type": "address", "internalType": "address"},
					{"name": "to", "type": "address", "internalType": "address"},
					{"name": "deadline", "type": "uint256", "internalType": "uint256"}
				]
			}
		],
		"outputs": [
			{"name": "amountOut", "type": "uint256", "internalType": "uint256"}
		],
		"stateMutability": "payable"
	}
]`

// NadFunDexRouterABI is the ABI for the nad.fun DexRouter.
// Used when token has graduated to DEX trading.
const NadFunDexRouterABI = `[
	{
		"type": "function",
		"name": "buy",
		"inputs": [
			{
				"name": "params",
				"type": "tuple",
				"internalType": "struct BuyParams",
				"components": [
					{"name": "amountOutMin", "type": "uint256", "internalType": "uint256"},
					{"name": "token", "type": "address", "internalType": "address"},
					{"name": "to", "type": "address", "internalType": "address"},
					{"name": "deadline", "type": "uint256", "internalType": "uint256"}
				]
			}
		],
		"outputs": [
			{"name": "amountOut", "type": "uint256", "internalType": "uint256"}
		],
		"stateMutability": "payable"
	}
]`

// BuyParams represents the parameters for a buy operation on nad.fun.
type BuyParams struct {
	AmountOutMin *big.Int       // Minimum tokens to receive
	Token        common.Address // Token to buy
	To           common.Address // Recipient address
	Deadline     *big.Int       // Transaction deadline timestamp
}

// NadFunLens provides read operations for the nad.fun Lens contract.
type NadFunLens struct {
	contract *bind.BoundContract
	address  common.Address
}

// NewNadFunLens creates a new Lens contract binding.
func NewNadFunLens(address common.Address, backend bind.ContractBackend) (*NadFunLens, error) {
	parsed, err := abi.JSON(strings.NewReader(NadFunLensABI))
	if err != nil {
		return nil, err
	}
	contract := bind.NewBoundContract(address, parsed, backend, backend, backend)
	return &NadFunLens{
		contract: contract,
		address:  address,
	}, nil
}

// GetAmountOut returns the router to use and expected output amount for a swap.
// isBuy should be true when buying tokens with MON.
func (l *NadFunLens) GetAmountOut(opts *bind.CallOpts, token common.Address, amountIn *big.Int, isBuy bool) (common.Address, *big.Int, error) {
	var out []interface{}
	err := l.contract.Call(opts, &out, "getAmountOut", token, amountIn, isBuy)
	if err != nil {
		return common.Address{}, nil, err
	}
	return out[0].(common.Address), out[1].(*big.Int), nil
}

// IsGraduated returns whether a token has graduated from bonding curve to DEX.
func (l *NadFunLens) IsGraduated(opts *bind.CallOpts, token common.Address) (bool, error) {
	var out []interface{}
	err := l.contract.Call(opts, &out, "isGraduated", token)
	if err != nil {
		return false, err
	}
	return out[0].(bool), nil
}

// IsLocked returns whether a token is locked (reached target, pending graduation).
func (l *NadFunLens) IsLocked(opts *bind.CallOpts, token common.Address) (bool, error) {
	var out []interface{}
	err := l.contract.Call(opts, &out, "isLocked", token)
	if err != nil {
		return false, err
	}
	return out[0].(bool), nil
}

// GetProgress returns the bonding curve progress in basis points (10000 = 100%).
func (l *NadFunLens) GetProgress(opts *bind.CallOpts, token common.Address) (*big.Int, error) {
	var out []interface{}
	err := l.contract.Call(opts, &out, "getProgress", token)
	if err != nil {
		return nil, err
	}
	return out[0].(*big.Int), nil
}

// NadFunRouter provides buy operations for nad.fun routers.
// Works for both BondingCurveRouter and DexRouter (same interface).
type NadFunRouter struct {
	contract *bind.BoundContract
	address  common.Address
}

// NewNadFunBondingCurveRouter creates a new BondingCurveRouter binding.
func NewNadFunBondingCurveRouter(address common.Address, backend bind.ContractBackend) (*NadFunRouter, error) {
	parsed, err := abi.JSON(strings.NewReader(NadFunBondingCurveRouterABI))
	if err != nil {
		return nil, err
	}
	contract := bind.NewBoundContract(address, parsed, backend, backend, backend)
	return &NadFunRouter{
		contract: contract,
		address:  address,
	}, nil
}

// NewNadFunDexRouter creates a new DexRouter binding.
func NewNadFunDexRouter(address common.Address, backend bind.ContractBackend) (*NadFunRouter, error) {
	parsed, err := abi.JSON(strings.NewReader(NadFunDexRouterABI))
	if err != nil {
		return nil, err
	}
	contract := bind.NewBoundContract(address, parsed, backend, backend, backend)
	return &NadFunRouter{
		contract: contract,
		address:  address,
	}, nil
}

// Address returns the router contract address.
func (r *NadFunRouter) Address() common.Address {
	return r.address
}

// Buy executes a buy operation, swapping MON for tokens.
// The MON amount is specified in opts.Value.
func (r *NadFunRouter) Buy(opts *bind.TransactOpts, params BuyParams) (*types.Transaction, error) {
	return r.contract.Transact(opts, "buy", params)
}
