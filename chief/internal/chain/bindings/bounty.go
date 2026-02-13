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
const BountyArenaABI = `[{"type":"function","name":"createBounty","inputs":[{"name":"question","type":"string"},{"name":"minRating","type":"int128"},{"name":"category","type":"string"},{"name":"difficulty","type":"uint8"},{"name":"deadline","type":"uint64"},{"name":"maxAgents","type":"uint8"},{"name":"reward","type":"uint256"},{"name":"baseAnswerFee","type":"uint256"}],"outputs":[{"name":"bountyId","type":"uint256"}],"stateMutability":"nonpayable"},{"type":"function","name":"joinBounty","inputs":[{"name":"bountyId","type":"uint256"},{"name":"agentId","type":"uint256"}],"outputs":[],"stateMutability":"nonpayable"},{"type":"function","name":"submitBountyAnswer","inputs":[{"name":"bountyId","type":"uint256"},{"name":"answer","type":"string"}],"outputs":[],"stateMutability":"nonpayable"},{"type":"function","name":"pickWinner","inputs":[{"name":"bountyId","type":"uint256"},{"name":"winner","type":"address"}],"outputs":[],"stateMutability":"nonpayable"},{"type":"function","name":"claimWinnerReward","inputs":[{"name":"bountyId","type":"uint256"}],"outputs":[],"stateMutability":"nonpayable"},{"type":"function","name":"claimProportional","inputs":[{"name":"bountyId","type":"uint256"}],"outputs":[],"stateMutability":"nonpayable"},{"type":"function","name":"claimRefund","inputs":[{"name":"bountyId","type":"uint256"}],"outputs":[],"stateMutability":"nonpayable"},{"type":"function","name":"getBountyState","inputs":[{"name":"bountyId","type":"uint256"}],"outputs":[{"name":"","type":"tuple","components":[{"name":"phase","type":"uint8"},{"name":"winner","type":"address"},{"name":"agentCount","type":"uint256"},{"name":"answeringAgentCount","type":"uint256"},{"name":"totalAnsweringReputation","type":"int128"},{"name":"rewardClaimed","type":"bool"}]}],"stateMutability":"view"},{"type":"function","name":"getBountyConfig","inputs":[{"name":"bountyId","type":"uint256"}],"outputs":[{"name":"","type":"tuple","components":[{"name":"creator","type":"address"},{"name":"reward","type":"uint256"},{"name":"baseAnswerFee","type":"uint256"},{"name":"minRating","type":"int128"},{"name":"deadline","type":"uint64"},{"name":"maxAgents","type":"uint8"},{"name":"question","type":"string"},{"name":"category","type":"string"},{"name":"difficulty","type":"uint8"}]}],"stateMutability":"view"},{"type":"function","name":"getBountyAgents","inputs":[{"name":"bountyId","type":"uint256"}],"outputs":[{"name":"","type":"address[]"}],"stateMutability":"view"},{"type":"function","name":"getAgentCount","inputs":[{"name":"bountyId","type":"uint256"}],"outputs":[{"name":"","type":"uint256"}],"stateMutability":"view"},{"type":"function","name":"nextBountyId","inputs":[],"outputs":[{"name":"","type":"uint256"}],"stateMutability":"view"},{"type":"function","name":"getCurrentAnswerFee","inputs":[{"name":"bountyId","type":"uint256"},{"name":"agent","type":"address"}],"outputs":[{"name":"","type":"uint256"}],"stateMutability":"view"},{"type":"function","name":"getClaimableAmount","inputs":[{"name":"bountyId","type":"uint256"},{"name":"agent","type":"address"}],"outputs":[{"name":"","type":"uint256"}],"stateMutability":"view"},{"type":"function","name":"MAX_ANSWER_ATTEMPTS","inputs":[],"outputs":[{"name":"","type":"uint256"}],"stateMutability":"view"},{"type":"function","name":"MAX_ANSWER_LENGTH","inputs":[],"outputs":[{"name":"","type":"uint256"}],"stateMutability":"view"},{"type":"event","name":"BountyCreated","inputs":[{"name":"bountyId","type":"uint256","indexed":true},{"name":"creator","type":"address","indexed":true},{"name":"reward","type":"uint256","indexed":false},{"name":"baseAnswerFee","type":"uint256","indexed":false},{"name":"question","type":"string","indexed":false},{"name":"category","type":"string","indexed":false},{"name":"difficulty","type":"uint8","indexed":false},{"name":"minRating","type":"int128","indexed":false},{"name":"deadline","type":"uint64","indexed":false},{"name":"maxAgents","type":"uint8","indexed":false}],"anonymous":false},{"type":"event","name":"AgentJoinedBounty","inputs":[{"name":"bountyId","type":"uint256","indexed":true},{"name":"agent","type":"address","indexed":true},{"name":"agentId","type":"uint256","indexed":false},{"name":"agentCount","type":"uint256","indexed":false},{"name":"snapshotReputation","type":"int128","indexed":false}],"anonymous":false},{"type":"event","name":"BountySettled","inputs":[{"name":"bountyId","type":"uint256","indexed":true},{"name":"winner","type":"address","indexed":true},{"name":"reward","type":"uint256","indexed":false}],"anonymous":false},{"type":"event","name":"WinnerRewardClaimed","inputs":[{"name":"bountyId","type":"uint256","indexed":true},{"name":"winner","type":"address","indexed":true},{"name":"amount","type":"uint256","indexed":false}],"anonymous":false},{"type":"event","name":"ProportionalClaimed","inputs":[{"name":"bountyId","type":"uint256","indexed":true},{"name":"agent","type":"address","indexed":true},{"name":"amount","type":"uint256","indexed":false}],"anonymous":false},{"type":"event","name":"RefundClaimed","inputs":[{"name":"bountyId","type":"uint256","indexed":true},{"name":"creator","type":"address","indexed":true},{"name":"amount","type":"uint256","indexed":false}],"anonymous":false},{"type":"event","name":"BountyAnswerSubmitted","inputs":[{"name":"bountyId","type":"uint256","indexed":true},{"name":"agent","type":"address","indexed":true},{"name":"answer","type":"string","indexed":false},{"name":"attemptNumber","type":"uint256","indexed":false},{"name":"neuronBurned","type":"uint256","indexed":false}],"anonymous":false},{"type":"event","name":"BountyNeuronBurned","inputs":[{"name":"bountyId","type":"uint256","indexed":true},{"name":"agent","type":"address","indexed":true},{"name":"amount","type":"uint256","indexed":false}],"anonymous":false}]`

// BountyState represents the on-chain state of a bounty.
type BountyState struct {
	Phase                    uint8
	Winner                   common.Address
	AgentCount               *big.Int
	AnsweringAgentCount      *big.Int
	TotalAnsweringReputation *big.Int // int128
	RewardClaimed            bool
}

// BountyConfig represents the on-chain configuration of a bounty.
type BountyConfig struct {
	Creator       common.Address
	Reward        *big.Int
	BaseAnswerFee *big.Int
	MinRating     *big.Int // int128
	Deadline      uint64
	MaxAgents     uint8
	Question      string
	Category      string
	Difficulty    uint8
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
	s := out[0].(struct {
		Phase                    uint8
		Winner                   common.Address
		AgentCount               *big.Int
		AnsweringAgentCount      *big.Int
		TotalAnsweringReputation *big.Int
		RewardClaimed            bool
	})
	return BountyState{
		Phase:                    s.Phase,
		Winner:                   s.Winner,
		AgentCount:               s.AgentCount,
		AnsweringAgentCount:      s.AnsweringAgentCount,
		TotalAnsweringReputation: s.TotalAnsweringReputation,
		RewardClaimed:            s.RewardClaimed,
	}, nil
}

// GetBountyConfig returns the configuration of a bounty.
func (c *BountyArenaCaller) GetBountyConfig(opts *bind.CallOpts, bountyId *big.Int) (BountyConfig, error) {
	var out []interface{}
	err := c.contract.Call(opts, &out, "getBountyConfig", bountyId)
	if err != nil {
		return BountyConfig{}, err
	}
	cfg := out[0].(struct {
		Creator       common.Address
		Reward        *big.Int
		BaseAnswerFee *big.Int
		MinRating     *big.Int
		Deadline      uint64
		MaxAgents     uint8
		Question      string
		Category      string
		Difficulty    uint8
	})
	return BountyConfig{
		Creator:       cfg.Creator,
		Reward:        cfg.Reward,
		BaseAnswerFee: cfg.BaseAnswerFee,
		MinRating:     cfg.MinRating,
		Deadline:      cfg.Deadline,
		MaxAgents:     cfg.MaxAgents,
		Question:      cfg.Question,
		Category:      cfg.Category,
		Difficulty:    cfg.Difficulty,
	}, nil
}

// GetBountyAgents returns the agents in a bounty.
func (c *BountyArenaCaller) GetBountyAgents(opts *bind.CallOpts, bountyId *big.Int) ([]common.Address, error) {
	var out []interface{}
	err := c.contract.Call(opts, &out, "getBountyAgents", bountyId)
	if err != nil {
		return nil, err
	}
	return out[0].([]common.Address), nil
}

// GetAgentCount returns the number of agents in a bounty.
func (c *BountyArenaCaller) GetAgentCount(opts *bind.CallOpts, bountyId *big.Int) (*big.Int, error) {
	var out []interface{}
	err := c.contract.Call(opts, &out, "getAgentCount", bountyId)
	if err != nil {
		return nil, err
	}
	return out[0].(*big.Int), nil
}

// PickWinner settles a bounty by picking the winner.
func (t *BountyArenaTransactor) PickWinner(opts *bind.TransactOpts, bountyId *big.Int, winner common.Address) (*types.Transaction, error) {
	return t.contract.Transact(opts, "pickWinner", bountyId, winner)
}
