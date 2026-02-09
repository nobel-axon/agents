// Code generated - DO NOT EDIT.
// This file is a simplified Go binding for AxonArena contract.

package bindings

import (
	"math/big"
	"strings"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/event"
)

// AxonArenaABI is the ABI of the AxonArena contract.
const AxonArenaABI = `[{"type":"constructor","inputs":[{"name":"_neuronToken","type":"address","internalType":"address"},{"name":"_treasury","type":"address","internalType":"address"},{"name":"_initialOperator","type":"address","internalType":"address"}],"stateMutability":"nonpayable"},{"type":"function","name":"addOperator","inputs":[{"name":"_operator","type":"address","internalType":"address"}],"outputs":[],"stateMutability":"nonpayable"},{"type":"function","name":"answerAttempts","inputs":[{"name":"","type":"uint256","internalType":"uint256"},{"name":"","type":"address","internalType":"address"}],"outputs":[{"name":"","type":"uint256","internalType":"uint256"}],"stateMutability":"view"},{"type":"function","name":"burnAllocation","inputs":[{"name":"","type":"address","internalType":"address"}],"outputs":[{"name":"","type":"uint256","internalType":"uint256"}],"stateMutability":"view"},{"type":"function","name":"cancelMatch","inputs":[{"name":"matchId","type":"uint256","internalType":"uint256"}],"outputs":[],"stateMutability":"nonpayable"},{"type":"function","name":"createMatch","inputs":[{"name":"entryFee","type":"uint256","internalType":"uint256"},{"name":"baseAnswerFee","type":"uint256","internalType":"uint256"},{"name":"queueDuration","type":"uint64","internalType":"uint64"},{"name":"answerDuration","type":"uint64","internalType":"uint64"},{"name":"minPlayers","type":"uint8","internalType":"uint8"},{"name":"maxPlayers","type":"uint8","internalType":"uint8"}],"outputs":[{"name":"matchId","type":"uint256","internalType":"uint256"}],"stateMutability":"nonpayable"},{"type":"function","name":"getCurrentAnswerFee","inputs":[{"name":"matchId","type":"uint256","internalType":"uint256"},{"name":"agent","type":"address","internalType":"address"}],"outputs":[{"name":"","type":"uint256","internalType":"uint256"}],"stateMutability":"view"},{"type":"function","name":"getMatchConfig","inputs":[{"name":"matchId","type":"uint256","internalType":"uint256"}],"outputs":[{"name":"config","type":"tuple","internalType":"struct AxonArena.MatchConfig","components":[{"name":"entryFee","type":"uint256","internalType":"uint256"},{"name":"baseAnswerFee","type":"uint256","internalType":"uint256"},{"name":"queueDeadline","type":"uint64","internalType":"uint64"},{"name":"answerDuration","type":"uint64","internalType":"uint64"},{"name":"minPlayers","type":"uint8","internalType":"uint8"},{"name":"maxPlayers","type":"uint8","internalType":"uint8"}]}],"stateMutability":"view"},{"type":"function","name":"getMatchPlayers","inputs":[{"name":"matchId","type":"uint256","internalType":"uint256"}],"outputs":[{"name":"","type":"address[]","internalType":"address[]"}],"stateMutability":"view"},{"type":"function","name":"getMatchQuestion","inputs":[{"name":"matchId","type":"uint256","internalType":"uint256"}],"outputs":[{"name":"question","type":"tuple","internalType":"struct AxonArena.MatchQuestion","components":[{"name":"questionText","type":"string","internalType":"string"},{"name":"category","type":"string","internalType":"string"},{"name":"formatHint","type":"string","internalType":"string"}]}],"stateMutability":"view"},{"type":"function","name":"getMatchState","inputs":[{"name":"matchId","type":"uint256","internalType":"uint256"}],"outputs":[{"name":"state","type":"tuple","internalType":"struct AxonArena.MatchState","components":[{"name":"pool","type":"uint256","internalType":"uint256"},{"name":"answerDeadline","type":"uint64","internalType":"uint64"},{"name":"phase","type":"uint8","internalType":"enum AxonArena.MatchPhase"},{"name":"difficulty","type":"uint8","internalType":"uint8"},{"name":"winner","type":"address","internalType":"address"},{"name":"answerHash","type":"bytes32","internalType":"bytes32"}]}],"stateMutability":"view"},{"type":"function","name":"getPlayerCount","inputs":[{"name":"matchId","type":"uint256","internalType":"uint256"}],"outputs":[{"name":"","type":"uint256","internalType":"uint256"}],"stateMutability":"view"},{"type":"function","name":"isOperator","inputs":[{"name":"_operator","type":"address","internalType":"address"}],"outputs":[{"name":"","type":"bool","internalType":"bool"}],"stateMutability":"view"},{"type":"function","name":"isPlayerInMatch","inputs":[{"name":"","type":"uint256","internalType":"uint256"},{"name":"","type":"address","internalType":"address"}],"outputs":[{"name":"","type":"bool","internalType":"bool"}],"stateMutability":"view"},{"type":"function","name":"joinQueue","inputs":[{"name":"matchId","type":"uint256","internalType":"uint256"}],"outputs":[],"stateMutability":"payable"},{"type":"function","name":"matchBurnTotal","inputs":[{"name":"","type":"uint256","internalType":"uint256"}],"outputs":[{"name":"","type":"uint256","internalType":"uint256"}],"stateMutability":"view"},{"type":"function","name":"neuronToken","inputs":[],"outputs":[{"name":"","type":"address","internalType":"contract INeuronToken"}],"stateMutability":"view"},{"type":"function","name":"nextMatchId","inputs":[],"outputs":[{"name":"","type":"uint256","internalType":"uint256"}],"stateMutability":"view"},{"type":"function","name":"operators","inputs":[{"name":"","type":"address","internalType":"address"}],"outputs":[{"name":"","type":"bool","internalType":"bool"}],"stateMutability":"view"},{"type":"function","name":"owner","inputs":[],"outputs":[{"name":"","type":"address","internalType":"address"}],"stateMutability":"view"},{"type":"function","name":"postQuestion","inputs":[{"name":"matchId","type":"uint256","internalType":"uint256"},{"name":"question","type":"string","internalType":"string"},{"name":"category","type":"string","internalType":"string"},{"name":"difficulty","type":"uint8","internalType":"uint8"},{"name":"formatHint","type":"string","internalType":"string"},{"name":"answerHash","type":"bytes32","internalType":"bytes32"}],"outputs":[],"stateMutability":"nonpayable"},{"type":"function","name":"refundMatch","inputs":[{"name":"matchId","type":"uint256","internalType":"uint256"}],"outputs":[],"stateMutability":"nonpayable"},{"type":"function","name":"removeOperator","inputs":[{"name":"_operator","type":"address","internalType":"address"}],"outputs":[],"stateMutability":"nonpayable"},{"type":"function","name":"revealAnswer","inputs":[{"name":"matchId","type":"uint256","internalType":"uint256"},{"name":"answer","type":"string","internalType":"string"},{"name":"salt","type":"bytes32","internalType":"bytes32"}],"outputs":[],"stateMutability":"nonpayable"},{"type":"function","name":"revealedAnswers","inputs":[{"name":"","type":"uint256","internalType":"uint256"}],"outputs":[{"name":"","type":"string","internalType":"string"}],"stateMutability":"view"},{"type":"function","name":"settleWinner","inputs":[{"name":"matchId","type":"uint256","internalType":"uint256"},{"name":"winner","type":"address","internalType":"address"}],"outputs":[],"stateMutability":"nonpayable"},{"type":"function","name":"startAnswerPeriod","inputs":[{"name":"matchId","type":"uint256","internalType":"uint256"}],"outputs":[],"stateMutability":"nonpayable"},{"type":"function","name":"startMatch","inputs":[{"name":"matchId","type":"uint256","internalType":"uint256"}],"outputs":[],"stateMutability":"nonpayable"},{"type":"function","name":"submitAnswer","inputs":[{"name":"matchId","type":"uint256","internalType":"uint256"},{"name":"answer","type":"string","internalType":"string"}],"outputs":[],"stateMutability":"nonpayable"},{"type":"function","name":"treasury","inputs":[],"outputs":[{"name":"","type":"address","internalType":"address"}],"stateMutability":"view"},{"type":"event","name":"AgentJoinedQueue","inputs":[{"name":"matchId","type":"uint256","indexed":true,"internalType":"uint256"},{"name":"agent","type":"address","indexed":true,"internalType":"address"},{"name":"playerCount","type":"uint256","indexed":false,"internalType":"uint256"},{"name":"poolTotal","type":"uint256","indexed":false,"internalType":"uint256"}],"anonymous":false},{"type":"event","name":"AnswerPeriodStarted","inputs":[{"name":"matchId","type":"uint256","indexed":true,"internalType":"uint256"},{"name":"startTime","type":"uint256","indexed":false,"internalType":"uint256"},{"name":"deadline","type":"uint256","indexed":false,"internalType":"uint256"}],"anonymous":false},{"type":"event","name":"AnswerRevealed","inputs":[{"name":"matchId","type":"uint256","indexed":true,"internalType":"uint256"},{"name":"answer","type":"string","indexed":false,"internalType":"string"},{"name":"salt","type":"bytes32","indexed":false,"internalType":"bytes32"}],"anonymous":false},{"type":"event","name":"AnswerSubmitted","inputs":[{"name":"matchId","type":"uint256","indexed":true,"internalType":"uint256"},{"name":"agent","type":"address","indexed":true,"internalType":"address"},{"name":"answer","type":"string","indexed":false,"internalType":"string"},{"name":"attemptNumber","type":"uint256","indexed":false,"internalType":"uint256"},{"name":"neuronBurned","type":"uint256","indexed":false,"internalType":"uint256"}],"anonymous":false},{"type":"event","name":"MatchCancelled","inputs":[{"name":"matchId","type":"uint256","indexed":true,"internalType":"uint256"}],"anonymous":false},{"type":"event","name":"MatchCreated","inputs":[{"name":"matchId","type":"uint256","indexed":true,"internalType":"uint256"},{"name":"entryFee","type":"uint256","indexed":false,"internalType":"uint256"},{"name":"baseAnswerFee","type":"uint256","indexed":false,"internalType":"uint256"},{"name":"queueDeadline","type":"uint64","indexed":false,"internalType":"uint64"},{"name":"minPlayers","type":"uint8","indexed":false,"internalType":"uint8"},{"name":"maxPlayers","type":"uint8","indexed":false,"internalType":"uint8"}],"anonymous":false},{"type":"event","name":"MatchRefunded","inputs":[{"name":"matchId","type":"uint256","indexed":true,"internalType":"uint256"},{"name":"playerCount","type":"uint256","indexed":false,"internalType":"uint256"},{"name":"refundPerPlayer","type":"uint256","indexed":false,"internalType":"uint256"}],"anonymous":false},{"type":"event","name":"MatchSettled","inputs":[{"name":"matchId","type":"uint256","indexed":true,"internalType":"uint256"},{"name":"winner","type":"address","indexed":true,"internalType":"address"},{"name":"winnerPrize","type":"uint256","indexed":false,"internalType":"uint256"},{"name":"treasuryFee","type":"uint256","indexed":false,"internalType":"uint256"},{"name":"burnAllocationAmount","type":"uint256","indexed":false,"internalType":"uint256"}],"anonymous":false},{"type":"event","name":"MatchStarted","inputs":[{"name":"matchId","type":"uint256","indexed":true,"internalType":"uint256"},{"name":"playerCount","type":"uint256","indexed":false,"internalType":"uint256"},{"name":"pool","type":"uint256","indexed":false,"internalType":"uint256"}],"anonymous":false},{"type":"event","name":"NeuronBurned","inputs":[{"name":"matchId","type":"uint256","indexed":true,"internalType":"uint256"},{"name":"agent","type":"address","indexed":true,"internalType":"address"},{"name":"amount","type":"uint256","indexed":false,"internalType":"uint256"}],"anonymous":false},{"type":"event","name":"QuestionRevealed","inputs":[{"name":"matchId","type":"uint256","indexed":true,"internalType":"uint256"},{"name":"question","type":"string","indexed":false,"internalType":"string"},{"name":"category","type":"string","indexed":false,"internalType":"string"},{"name":"difficulty","type":"uint8","indexed":false,"internalType":"uint8"},{"name":"formatHint","type":"string","indexed":false,"internalType":"string"},{"name":"answerHash","type":"bytes32","indexed":false,"internalType":"bytes32"}],"anonymous":false},{"type":"function","name":"claimBurnAllocationFor","inputs":[{"name":"winner","type":"address","internalType":"address"}],"outputs":[],"stateMutability":"nonpayable"},{"type":"event","name":"BurnAllocationClaimed","inputs":[{"name":"operator","type":"address","indexed":true,"internalType":"address"},{"name":"winner","type":"address","indexed":true,"internalType":"address"},{"name":"amount","type":"uint256","indexed":false,"internalType":"uint256"}],"anonymous":false}]`

// MatchPhase represents the phase of a match.
type MatchPhase uint8

// Note: These values do NOT match the contract's phase enum directly.
// Use chainPhaseToInternal() in recovery.go for correct mapping.
// Contract: Queue=0, QuestionRevealed=1, AnswerPeriod=2, Settled=3, Refunded=4
const (
	PhaseNone MatchPhase = iota
	PhaseQueue
	PhaseActive
	PhaseAnswerPeriod
	PhaseSettled
	PhaseRefunded
	PhaseCancelled
)

// MatchConfig represents the configuration of a match.
type MatchConfig struct {
	EntryFee       *big.Int
	BaseAnswerFee  *big.Int
	QueueDeadline  uint64
	AnswerDuration uint64
	MinPlayers     uint8
	MaxPlayers     uint8
}

// MatchState represents the state of a match.
type MatchState struct {
	Pool           *big.Int
	AnswerDeadline uint64
	Phase          uint8
	Difficulty     uint8
	Winner         common.Address
	AnswerHash     [32]byte
}

// MatchQuestion represents the question for a match.
type MatchQuestion struct {
	QuestionText string
	Category     string
	FormatHint   string
}

// AxonArena is a Go binding for the AxonArena contract.
type AxonArena struct {
	AxonArenaCaller
	AxonArenaTransactor
	AxonArenaFilterer
}

// AxonArenaCaller is the read-only binding to the contract.
type AxonArenaCaller struct {
	contract *bind.BoundContract
}

// AxonArenaTransactor is the write-only binding to the contract.
type AxonArenaTransactor struct {
	contract *bind.BoundContract
}

// AxonArenaFilterer is the event filtering binding to the contract.
type AxonArenaFilterer struct {
	contract *bind.BoundContract
}

// NewAxonArena creates a new instance of AxonArena.
func NewAxonArena(address common.Address, backend bind.ContractBackend) (*AxonArena, error) {
	parsed, err := abi.JSON(strings.NewReader(AxonArenaABI))
	if err != nil {
		return nil, err
	}
	contract := bind.NewBoundContract(address, parsed, backend, backend, backend)
	return &AxonArena{
		AxonArenaCaller:     AxonArenaCaller{contract: contract},
		AxonArenaTransactor: AxonArenaTransactor{contract: contract},
		AxonArenaFilterer:   AxonArenaFilterer{contract: contract},
	}, nil
}

// NextMatchId returns the next match ID.
func (c *AxonArenaCaller) NextMatchId(opts *bind.CallOpts) (*big.Int, error) {
	var out []interface{}
	err := c.contract.Call(opts, &out, "nextMatchId")
	if err != nil {
		return nil, err
	}
	return out[0].(*big.Int), nil
}

// GetMatchState returns the state of a match.
func (c *AxonArenaCaller) GetMatchState(opts *bind.CallOpts, matchId *big.Int) (MatchState, error) {
	var out []interface{}
	err := c.contract.Call(opts, &out, "getMatchState", matchId)
	if err != nil {
		return MatchState{}, err
	}
	// Parse the tuple (json tags required to match go-ethereum's ABI unpacking)
	result := out[0].(struct {
		Pool           *big.Int       `json:"pool"`
		AnswerDeadline uint64         `json:"answerDeadline"`
		Phase          uint8          `json:"phase"`
		Difficulty     uint8          `json:"difficulty"`
		Winner         common.Address `json:"winner"`
		AnswerHash     [32]byte       `json:"answerHash"`
	})
	return MatchState{
		Pool:           result.Pool,
		AnswerDeadline: result.AnswerDeadline,
		Phase:          result.Phase,
		Difficulty:     result.Difficulty,
		Winner:         result.Winner,
		AnswerHash:     result.AnswerHash,
	}, nil
}

// GetMatchConfig returns the configuration of a match.
func (c *AxonArenaCaller) GetMatchConfig(opts *bind.CallOpts, matchId *big.Int) (MatchConfig, error) {
	var out []interface{}
	err := c.contract.Call(opts, &out, "getMatchConfig", matchId)
	if err != nil {
		return MatchConfig{}, err
	}
	result := out[0].(struct {
		EntryFee       *big.Int `json:"entryFee"`
		BaseAnswerFee  *big.Int `json:"baseAnswerFee"`
		QueueDeadline  uint64   `json:"queueDeadline"`
		AnswerDuration uint64   `json:"answerDuration"`
		MinPlayers     uint8    `json:"minPlayers"`
		MaxPlayers     uint8    `json:"maxPlayers"`
	})
	return MatchConfig{
		EntryFee:       result.EntryFee,
		BaseAnswerFee:  result.BaseAnswerFee,
		QueueDeadline:  result.QueueDeadline,
		AnswerDuration: result.AnswerDuration,
		MinPlayers:     result.MinPlayers,
		MaxPlayers:     result.MaxPlayers,
	}, nil
}

// GetMatchPlayers returns the players in a match.
func (c *AxonArenaCaller) GetMatchPlayers(opts *bind.CallOpts, matchId *big.Int) ([]common.Address, error) {
	var out []interface{}
	err := c.contract.Call(opts, &out, "getMatchPlayers", matchId)
	if err != nil {
		return nil, err
	}
	return out[0].([]common.Address), nil
}

// GetMatchQuestion returns the question for a match.
func (c *AxonArenaCaller) GetMatchQuestion(opts *bind.CallOpts, matchId *big.Int) (MatchQuestion, error) {
	var out []interface{}
	err := c.contract.Call(opts, &out, "getMatchQuestion", matchId)
	if err != nil {
		return MatchQuestion{}, err
	}
	result := out[0].(struct {
		QuestionText string `json:"questionText"`
		Category     string `json:"category"`
		FormatHint   string `json:"formatHint"`
	})
	return MatchQuestion{
		QuestionText: result.QuestionText,
		Category:     result.Category,
		FormatHint:   result.FormatHint,
	}, nil
}

// GetPlayerCount returns the number of players in a match.
func (c *AxonArenaCaller) GetPlayerCount(opts *bind.CallOpts, matchId *big.Int) (*big.Int, error) {
	var out []interface{}
	err := c.contract.Call(opts, &out, "getPlayerCount", matchId)
	if err != nil {
		return nil, err
	}
	return out[0].(*big.Int), nil
}

// IsOperator checks if an address is an operator.
func (c *AxonArenaCaller) IsOperator(opts *bind.CallOpts, operator common.Address) (bool, error) {
	var out []interface{}
	err := c.contract.Call(opts, &out, "isOperator", operator)
	if err != nil {
		return false, err
	}
	return out[0].(bool), nil
}

// NeuronToken returns the NEURON token address.
func (c *AxonArenaCaller) NeuronToken(opts *bind.CallOpts) (common.Address, error) {
	var out []interface{}
	err := c.contract.Call(opts, &out, "neuronToken")
	if err != nil {
		return common.Address{}, err
	}
	return out[0].(common.Address), nil
}

// Treasury returns the treasury address.
func (c *AxonArenaCaller) Treasury(opts *bind.CallOpts) (common.Address, error) {
	var out []interface{}
	err := c.contract.Call(opts, &out, "treasury")
	if err != nil {
		return common.Address{}, err
	}
	return out[0].(common.Address), nil
}

// BurnAllocation returns the accumulated burn allocation for an address.
func (c *AxonArenaCaller) BurnAllocation(opts *bind.CallOpts, winner common.Address) (*big.Int, error) {
	var out []interface{}
	err := c.contract.Call(opts, &out, "burnAllocation", winner)
	if err != nil {
		return nil, err
	}
	return out[0].(*big.Int), nil
}

// CreateMatch creates a new match.
func (t *AxonArenaTransactor) CreateMatch(opts *bind.TransactOpts, entryFee *big.Int, baseAnswerFee *big.Int, queueDuration uint64, answerDuration uint64, minPlayers uint8, maxPlayers uint8) (*types.Transaction, error) {
	return t.contract.Transact(opts, "createMatch", entryFee, baseAnswerFee, queueDuration, answerDuration, minPlayers, maxPlayers)
}

// PostQuestion posts a question to a match.
func (t *AxonArenaTransactor) PostQuestion(opts *bind.TransactOpts, matchId *big.Int, question string, category string, difficulty uint8, formatHint string, answerHash [32]byte) (*types.Transaction, error) {
	return t.contract.Transact(opts, "postQuestion", matchId, question, category, difficulty, formatHint, answerHash)
}

// StartMatch starts a match.
func (t *AxonArenaTransactor) StartMatch(opts *bind.TransactOpts, matchId *big.Int) (*types.Transaction, error) {
	return t.contract.Transact(opts, "startMatch", matchId)
}

// StartAnswerPeriod starts the answer period for a match.
func (t *AxonArenaTransactor) StartAnswerPeriod(opts *bind.TransactOpts, matchId *big.Int) (*types.Transaction, error) {
	return t.contract.Transact(opts, "startAnswerPeriod", matchId)
}

// SettleWinner settles a match with the given winner.
func (t *AxonArenaTransactor) SettleWinner(opts *bind.TransactOpts, matchId *big.Int, winner common.Address) (*types.Transaction, error) {
	return t.contract.Transact(opts, "settleWinner", matchId, winner)
}

// RevealAnswer reveals the answer for a match.
func (t *AxonArenaTransactor) RevealAnswer(opts *bind.TransactOpts, matchId *big.Int, answer string, salt [32]byte) (*types.Transaction, error) {
	return t.contract.Transact(opts, "revealAnswer", matchId, answer, salt)
}

// RefundMatch refunds all players in a match.
func (t *AxonArenaTransactor) RefundMatch(opts *bind.TransactOpts, matchId *big.Int) (*types.Transaction, error) {
	return t.contract.Transact(opts, "refundMatch", matchId)
}

// CancelMatch cancels a match.
func (t *AxonArenaTransactor) CancelMatch(opts *bind.TransactOpts, matchId *big.Int) (*types.Transaction, error) {
	return t.contract.Transact(opts, "cancelMatch", matchId)
}

// ClaimBurnAllocationFor claims the burn allocation on behalf of a winner.
func (t *AxonArenaTransactor) ClaimBurnAllocationFor(opts *bind.TransactOpts, winner common.Address) (*types.Transaction, error) {
	return t.contract.Transact(opts, "claimBurnAllocationFor", winner)
}

// Events

// AxonArenaMatchCreated represents a MatchCreated event.
type AxonArenaMatchCreated struct {
	MatchId       *big.Int
	EntryFee      *big.Int
	BaseAnswerFee *big.Int
	QueueDeadline uint64
	MinPlayers    uint8
	MaxPlayers    uint8
	Raw           types.Log
}

// AxonArenaMatchStarted represents a MatchStarted event.
type AxonArenaMatchStarted struct {
	MatchId     *big.Int
	PlayerCount *big.Int
	Pool        *big.Int
	Raw         types.Log
}

// AxonArenaAnswerPeriodStarted represents an AnswerPeriodStarted event.
type AxonArenaAnswerPeriodStarted struct {
	MatchId   *big.Int
	StartTime *big.Int
	Deadline  *big.Int
	Raw       types.Log
}

// AxonArenaAnswerSubmitted represents an AnswerSubmitted event.
type AxonArenaAnswerSubmitted struct {
	MatchId       *big.Int
	Agent         common.Address
	Answer        string
	AttemptNumber *big.Int
	NeuronBurned  *big.Int
	Raw           types.Log
}

// AxonArenaMatchSettled represents a MatchSettled event.
type AxonArenaMatchSettled struct {
	MatchId              *big.Int
	Winner               common.Address
	WinnerPrize          *big.Int
	TreasuryFee          *big.Int
	BurnAllocationAmount *big.Int
	Raw                  types.Log
}

// AxonArenaMatchRefunded represents a MatchRefunded event.
type AxonArenaMatchRefunded struct {
	MatchId         *big.Int
	PlayerCount     *big.Int
	RefundPerPlayer *big.Int
	Raw             types.Log
}

// AxonArenaMatchCancelled represents a MatchCancelled event.
type AxonArenaMatchCancelled struct {
	MatchId *big.Int
	Raw     types.Log
}

// WatchMatchCreated watches for MatchCreated events.
func (f *AxonArenaFilterer) WatchMatchCreated(opts *bind.WatchOpts, sink chan<- *AxonArenaMatchCreated, matchId []*big.Int) (event.Subscription, error) {
	var matchIdRule []interface{}
	for _, item := range matchId {
		matchIdRule = append(matchIdRule, item)
	}
	logs, sub, err := f.contract.WatchLogs(opts, "MatchCreated", matchIdRule)
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				event := new(AxonArenaMatchCreated)
				if err := f.contract.UnpackLog(event, "MatchCreated", log); err != nil {
					return err
				}
				event.Raw = log
				select {
				case sink <- event:
				case <-quit:
					return nil
				}
			case <-quit:
				return nil
			}
		}
	}), nil
}

// FilterMatchCreated filters MatchCreated events.
func (f *AxonArenaFilterer) FilterMatchCreated(opts *bind.FilterOpts, matchId []*big.Int) (*AxonArenaMatchCreatedIterator, error) {
	var matchIdRule []interface{}
	for _, item := range matchId {
		matchIdRule = append(matchIdRule, item)
	}
	logs, sub, err := f.contract.FilterLogs(opts, "MatchCreated", matchIdRule)
	if err != nil {
		return nil, err
	}
	return &AxonArenaMatchCreatedIterator{contract: f.contract, logs: logs, sub: sub}, nil
}

// AxonArenaMatchCreatedIterator is an iterator for MatchCreated events.
type AxonArenaMatchCreatedIterator struct {
	Event    *AxonArenaMatchCreated
	contract *bind.BoundContract
	logs     chan types.Log
	sub      ethereum.Subscription
	done     bool
	fail     error
}

// Next returns true if there is another event.
func (it *AxonArenaMatchCreatedIterator) Next() bool {
	if it.fail != nil {
		return false
	}
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(AxonArenaMatchCreated)
			if err := it.contract.UnpackLog(it.Event, "MatchCreated", log); err != nil {
				it.fail = err
				return false
			}
			it.Event.Raw = log
			return true
		default:
			return false
		}
	}
	select {
	case log := <-it.logs:
		it.Event = new(AxonArenaMatchCreated)
		if err := it.contract.UnpackLog(it.Event, "MatchCreated", log); err != nil {
			it.fail = err
			return false
		}
		it.Event.Raw = log
		return true
	case err := <-it.sub.Err():
		it.done = true
		it.fail = err
		return it.Next()
	}
}

// Error returns the error.
func (it *AxonArenaMatchCreatedIterator) Error() error {
	return it.fail
}

// Close closes the iterator.
func (it *AxonArenaMatchCreatedIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// WatchAnswerSubmitted watches for AnswerSubmitted events.
func (f *AxonArenaFilterer) WatchAnswerSubmitted(opts *bind.WatchOpts, sink chan<- *AxonArenaAnswerSubmitted, matchId []*big.Int, agent []common.Address) (event.Subscription, error) {
	var matchIdRule []interface{}
	for _, item := range matchId {
		matchIdRule = append(matchIdRule, item)
	}
	var agentRule []interface{}
	for _, item := range agent {
		agentRule = append(agentRule, item)
	}
	logs, sub, err := f.contract.WatchLogs(opts, "AnswerSubmitted", matchIdRule, agentRule)
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				event := new(AxonArenaAnswerSubmitted)
				if err := f.contract.UnpackLog(event, "AnswerSubmitted", log); err != nil {
					return err
				}
				event.Raw = log
				select {
				case sink <- event:
				case <-quit:
					return nil
				}
			case <-quit:
				return nil
			}
		}
	}), nil
}
