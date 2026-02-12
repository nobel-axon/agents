// Code generated - DO NOT EDIT.
// This file is a simplified Go binding for the BountyArena contract.

package bindings

import (
	"math/big"
	"strings"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
)

// BountyArenaABI is the ABI of the BountyArena contract.
const BountyArenaABI = `[{"type":"function","name":"createBounty","inputs":[{"name":"question","type":"string","internalType":"string"},{"name":"category","type":"string","internalType":"string"},{"name":"difficulty","type":"uint8","internalType":"uint8"},{"name":"entryFee","type":"uint256","internalType":"uint256"},{"name":"deadline","type":"uint64","internalType":"uint64"},{"name":"maxParticipants","type":"uint8","internalType":"uint8"},{"name":"minRating","type":"uint256","internalType":"uint256"}],"outputs":[{"name":"bountyId","type":"uint256","internalType":"uint256"}],"stateMutability":"payable"},{"type":"function","name":"joinBounty","inputs":[{"name":"bountyId","type":"uint256","internalType":"uint256"}],"outputs":[],"stateMutability":"payable"},{"type":"function","name":"submitBountyAnswer","inputs":[{"name":"bountyId","type":"uint256","internalType":"uint256"},{"name":"answer","type":"string","internalType":"string"}],"outputs":[],"stateMutability":"nonpayable"},{"type":"function","name":"settleBounty","inputs":[{"name":"bountyId","type":"uint256","internalType":"uint256"},{"name":"winner","type":"address","internalType":"address"}],"outputs":[],"stateMutability":"nonpayable"},{"type":"function","name":"refundBounty","inputs":[{"name":"bountyId","type":"uint256","internalType":"uint256"}],"outputs":[],"stateMutability":"nonpayable"},{"type":"function","name":"getBountyState","inputs":[{"name":"bountyId","type":"uint256","internalType":"uint256"}],"outputs":[{"name":"state","type":"tuple","internalType":"struct BountyArena.BountyState","components":[{"name":"creator","type":"address","internalType":"address"},{"name":"pool","type":"uint256","internalType":"uint256"},{"name":"entryFee","type":"uint256","internalType":"uint256"},{"name":"deadline","type":"uint64","internalType":"uint64"},{"name":"phase","type":"uint8","internalType":"uint8"},{"name":"playerCount","type":"uint8","internalType":"uint8"},{"name":"maxParticipants","type":"uint8","internalType":"uint8"},{"name":"minRating","type":"uint256","internalType":"uint256"},{"name":"winner","type":"address","internalType":"address"}]}],"stateMutability":"view"},{"type":"function","name":"getBountyPlayers","inputs":[{"name":"bountyId","type":"uint256","internalType":"uint256"}],"outputs":[{"name":"","type":"address[]","internalType":"address[]"}],"stateMutability":"view"},{"type":"function","name":"getBountyQuestion","inputs":[{"name":"bountyId","type":"uint256","internalType":"uint256"}],"outputs":[{"name":"question","type":"string","internalType":"string"},{"name":"category","type":"string","internalType":"string"},{"name":"difficulty","type":"uint8","internalType":"uint8"}],"stateMutability":"view"},{"type":"function","name":"nextBountyId","inputs":[],"outputs":[{"name":"","type":"uint256","internalType":"uint256"}],"stateMutability":"view"},{"type":"event","name":"BountyCreated","inputs":[{"name":"bountyId","type":"uint256","indexed":true,"internalType":"uint256"},{"name":"creator","type":"address","indexed":true,"internalType":"address"},{"name":"entryFee","type":"uint256","indexed":false,"internalType":"uint256"},{"name":"deadline","type":"uint64","indexed":false,"internalType":"uint64"},{"name":"minRating","type":"uint256","indexed":false,"internalType":"uint256"}],"anonymous":false},{"type":"event","name":"AgentJoinedBounty","inputs":[{"name":"bountyId","type":"uint256","indexed":true,"internalType":"uint256"},{"name":"agent","type":"address","indexed":true,"internalType":"address"},{"name":"playerCount","type":"uint256","indexed":false,"internalType":"uint256"}],"anonymous":false},{"type":"event","name":"BountyAnswerSubmitted","inputs":[{"name":"bountyId","type":"uint256","indexed":true,"internalType":"uint256"},{"name":"agent","type":"address","indexed":true,"internalType":"address"},{"name":"answer","type":"string","indexed":false,"internalType":"string"}],"anonymous":false},{"type":"event","name":"BountySettled","inputs":[{"name":"bountyId","type":"uint256","indexed":true,"internalType":"uint256"},{"name":"winner","type":"address","indexed":true,"internalType":"address"},{"name":"prize","type":"uint256","indexed":false,"internalType":"uint256"}],"anonymous":false},{"type":"event","name":"BountyRefunded","inputs":[{"name":"bountyId","type":"uint256","indexed":true,"internalType":"uint256"},{"name":"playerCount","type":"uint256","indexed":false,"internalType":"uint256"}],"anonymous":false}]`

// BountyState represents the on-chain state of a bounty.
type BountyState struct {
	Creator         common.Address
	Pool            *big.Int
	EntryFee        *big.Int
	Deadline        uint64
	Phase           uint8
	PlayerCount     uint8
	MaxParticipants uint8
	MinRating       *big.Int
	Winner          common.Address
}

// BountyArena is a Go binding for the BountyArena contract.
type BountyArena struct {
	BountyArenaCaller
	BountyArenaTransactor
}

// BountyArenaCaller is the read-only binding.
type BountyArenaCaller struct {
	contract *bind.BoundContract
}

// BountyArenaTransactor is the write-only binding.
type BountyArenaTransactor struct {
	contract *bind.BoundContract
}

// NewBountyArena creates a new instance of BountyArena.
func NewBountyArena(address common.Address, backend bind.ContractBackend) (*BountyArena, error) {
	parsed, err := abi.JSON(strings.NewReader(BountyArenaABI))
	if err != nil {
		return nil, err
	}
	contract := bind.NewBoundContract(address, parsed, backend, backend, backend)
	return &BountyArena{
		BountyArenaCaller:     BountyArenaCaller{contract: contract},
		BountyArenaTransactor: BountyArenaTransactor{contract: contract},
	}, nil
}

// NextBountyId returns the next bounty ID.
func (c *BountyArenaCaller) NextBountyId(opts *bind.CallOpts) (*big.Int, error) {
	var out []interface{}
	err := c.contract.Call(opts, &out, "nextBountyId")
	if err != nil {
		return nil, err
	}
	return out[0].(*big.Int), nil
}

// GetBountyState returns the state of a bounty.
func (c *BountyArenaCaller) GetBountyState(opts *bind.CallOpts, bountyId *big.Int) (BountyState, error) {
	var out []interface{}
	err := c.contract.Call(opts, &out, "getBountyState", bountyId)
	if err != nil {
		return BountyState{}, err
	}
	result := out[0].(struct {
		Creator         common.Address `json:"creator"`
		Pool            *big.Int       `json:"pool"`
		EntryFee        *big.Int       `json:"entryFee"`
		Deadline        uint64         `json:"deadline"`
		Phase           uint8          `json:"phase"`
		PlayerCount     uint8          `json:"playerCount"`
		MaxParticipants uint8          `json:"maxParticipants"`
		MinRating       *big.Int       `json:"minRating"`
		Winner          common.Address `json:"winner"`
	})
	return BountyState{
		Creator:         result.Creator,
		Pool:            result.Pool,
		EntryFee:        result.EntryFee,
		Deadline:        result.Deadline,
		Phase:           result.Phase,
		PlayerCount:     result.PlayerCount,
		MaxParticipants: result.MaxParticipants,
		MinRating:       result.MinRating,
		Winner:          result.Winner,
	}, nil
}

// GetBountyPlayers returns the players in a bounty.
func (c *BountyArenaCaller) GetBountyPlayers(opts *bind.CallOpts, bountyId *big.Int) ([]common.Address, error) {
	var out []interface{}
	err := c.contract.Call(opts, &out, "getBountyPlayers", bountyId)
	if err != nil {
		return nil, err
	}
	return out[0].([]common.Address), nil
}

// GetBountyQuestion returns the question for a bounty.
func (c *BountyArenaCaller) GetBountyQuestion(opts *bind.CallOpts, bountyId *big.Int) (string, string, uint8, error) {
	var out []interface{}
	err := c.contract.Call(opts, &out, "getBountyQuestion", bountyId)
	if err != nil {
		return "", "", 0, err
	}
	return out[0].(string), out[1].(string), out[2].(uint8), nil
}

// SettleBounty settles a bounty with the given winner.
func (t *BountyArenaTransactor) SettleBounty(opts *bind.TransactOpts, bountyId *big.Int, winner common.Address) (*types.Transaction, error) {
	return t.contract.Transact(opts, "settleBounty", bountyId, winner)
}

// RefundBounty refunds all players in a bounty.
func (t *BountyArenaTransactor) RefundBounty(opts *bind.TransactOpts, bountyId *big.Int) (*types.Transaction, error) {
	return t.contract.Transact(opts, "refundBounty", bountyId)
}
