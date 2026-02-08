// Package config provides configuration loading for axon-chief.
package config

import (
	"math/big"
	"os"
	"strconv"
	"time"

	"gopkg.in/yaml.v3"
)

// Config holds all configuration for the chief service.
type Config struct {
	// Server settings
	Port      string `yaml:"port"`
	AdminPort string `yaml:"adminPort"`

	// Chain settings
	Chain ChainConfig `yaml:"chain"`

	// Judge agent settings
	Judges JudgeConfig `yaml:"judges"`

	// Match defaults
	Match MatchConfig `yaml:"match"`

	// Watcher settings
	Watcher WatcherConfig `yaml:"watcher"`

	// Admin settings
	AdminToken string `yaml:"adminToken"`

	// nad.fun swap integration
	NadFun NadFunConfig `yaml:"nadFun"`

	// Server (axon-server) settings for reporter
	Server ServerConfig `yaml:"server"`
}

// ServerConfig holds axon-server connection settings for the reporter.
type ServerConfig struct {
	URL            string `yaml:"url"`
	InternalSecret string `yaml:"internalSecret"`
}

// ChainConfig holds blockchain connection settings.
type ChainConfig struct {
	RPCURL            string `yaml:"rpcUrl"`
	ChainID           int64  `yaml:"chainId"`
	OperatorKey       string `yaml:"operatorKey"`
	ArenaAddress      string `yaml:"arenaAddress"`
	NeuronAddress     string `yaml:"neuronAddress"`
	GasLimitBuffer    uint64 `yaml:"gasLimitBuffer"`
	MaxGasPrice       string `yaml:"maxGasPrice"`
	TxRetryCount      int    `yaml:"txRetryCount"`
	TxRetryIntervalMs int    `yaml:"txRetryIntervalMs"`
}

// JudgeConfig holds agent endpoint settings for each role.
type JudgeConfig struct {
	PhilosopherEndpoint string   `yaml:"philosopherEndpoint"`
	DirectorEndpoint    string   `yaml:"directorEndpoint"`
	JudgeEndpoints      []string `yaml:"judgeEndpoints"`
	TimeoutMs           int      `yaml:"timeoutMs"`
	ConsensusThreshold  int      `yaml:"consensusThreshold"` // Number of agreeing judges required (default 2)
	RetryCount          int      `yaml:"retryCount"`
}

// MatchConfig holds match creation defaults.
type MatchConfig struct {
	EntryFee          string        `yaml:"entryFee"`          // In wei
	BaseAnswerFee     string        `yaml:"baseAnswerFee"`     // In NEURON wei
	QueueDuration     time.Duration `yaml:"queueDuration"`     // Duration for queue phase
	AnswerDuration    time.Duration `yaml:"answerDuration"`    // Duration for answer phase
	GapDuration       time.Duration `yaml:"gapDuration"`       // Duration for reveal phase (question + judges visible)
	MinPlayers        uint8         `yaml:"minPlayers"`
	MaxPlayers        uint8         `yaml:"maxPlayers"`
	AutoCreate        bool          `yaml:"autoCreate"`        // Auto-create next match
	Cooldown          time.Duration `yaml:"cooldown"`          // Cooldown between matches
	RegistrationGrace time.Duration `yaml:"registrationGrace"` // Grace period after minPlayers reached before auto-start
}

// WatcherConfig holds watcher settings.
type WatcherConfig struct {
	TimeoutCheckInterval   time.Duration `yaml:"timeoutCheckInterval"`
	StaleCheckInterval     time.Duration `yaml:"staleCheckInterval"`
	StaleThreshold         time.Duration `yaml:"staleThreshold"`
	EventPollInterval      time.Duration `yaml:"eventPollInterval"`
	PonderHealthThreshold  time.Duration `yaml:"ponderHealthThreshold"`
	PonderLagBlocks        int           `yaml:"ponderLagBlocks"`
	DirectChainPolling     bool          `yaml:"directChainPolling"` // Enable direct eth_getLogs polling (default false, use Ponder)
}

// NadFunConfig holds nad.fun swap integration settings.
type NadFunConfig struct {
	// Contract addresses (Monad mainnet defaults)
	BondingCurveRouterAddress string `yaml:"bondingCurveRouterAddress"`
	DexRouterAddress          string `yaml:"dexRouterAddress"`
	LensAddress               string `yaml:"lensAddress"`
	WMONAddress               string `yaml:"wmonAddress"`

	// Swap parameters
	SwapSlippageBps      int `yaml:"swapSlippageBps"`      // Slippage tolerance in basis points (100 = 1%)
	SwapDeadlineSeconds  int `yaml:"swapDeadlineSeconds"`  // Transaction deadline in seconds
	BurnSwapRetries      int `yaml:"burnSwapRetries"`      // Max retry attempts for burn swap
	BurnSwapRetryDelayMs int `yaml:"burnSwapRetryDelayMs"` // Delay between retries in ms

	// Fallback settings
	TreasuryFallback bool `yaml:"treasuryFallback"` // Send to treasury if all swaps fail
}

// Default returns a Config with default values.
func Default() *Config {
	return &Config{
		Port:      "9100",
		AdminPort: "9101",
		Chain: ChainConfig{
			RPCURL:            "https://testnet.monad.xyz",
			ChainID:           10143,
			GasLimitBuffer:    50000,
			MaxGasPrice:       "100000000000", // 100 gwei
			TxRetryCount:      5,
			TxRetryIntervalMs: 1000,
		},
		Judges: JudgeConfig{
			PhilosopherEndpoint: "http://localhost:9001",
			DirectorEndpoint:    "http://localhost:9002",
			JudgeEndpoints:      []string{"http://localhost:9003", "http://localhost:9004", "http://localhost:9005"},
			TimeoutMs:           30000,
			ConsensusThreshold:  2,
			RetryCount:          3,
		},
		Match: MatchConfig{
			EntryFee:          "100000000000000000",   // 0.1 MON
			BaseAnswerFee:     "1000000000000000000",  // 1 NEURON
			QueueDuration:     5 * time.Minute,
			AnswerDuration:    3 * time.Minute,
			GapDuration:       10 * time.Second,       // 10s for reveal phase
			MinPlayers:        2,
			MaxPlayers:        10,
			AutoCreate:        true,
			Cooldown:          30 * time.Second,
			RegistrationGrace: 30 * time.Second,       // 30s grace period after minPlayers reached
		},
		Watcher: WatcherConfig{
			TimeoutCheckInterval:   10 * time.Second,
			StaleCheckInterval:     30 * time.Second,
			StaleThreshold:         5 * time.Minute,
			EventPollInterval:      500 * time.Millisecond,
			PonderHealthThreshold:  30 * time.Second,
			PonderLagBlocks:        10,
		},
		AdminToken: "",
		Server: ServerConfig{
			URL:            "http://localhost:8080",
			InternalSecret: "",
		},
		NadFun: NadFunConfig{
			// Monad mainnet contract addresses
			BondingCurveRouterAddress: "0x6F6B8F1a20703309951a5127c45B49b1CD981A22",
			DexRouterAddress:          "0x0B79d71AE99528D1dB24A4148b5f4F865cc2b137",
			LensAddress:               "0x7e78A8DE94f21804F7a17F4E8BF9EC2c872187ea",
			WMONAddress:               "0x3bd359C1119dA7Da1D913D1C4D2B7c461115433A",

			// Swap parameters
			SwapSlippageBps:      100, // 1% slippage
			SwapDeadlineSeconds:  300, // 5 minute deadline
			BurnSwapRetries:      3,
			BurnSwapRetryDelayMs: 5000, // 5 seconds

			// Fallback
			TreasuryFallback: true,
		},
	}
}

// Load loads configuration from a YAML file and environment variables.
// Environment variables take precedence over file values.
func Load(path string) (*Config, error) {
	cfg := Default()

	// Load from file if provided
	if path != "" {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		if err := yaml.Unmarshal(data, cfg); err != nil {
			return nil, err
		}
	}

	// Override with environment variables
	if v := os.Getenv("PORT"); v != "" {
		cfg.Port = v
	}
	if v := os.Getenv("ADMIN_PORT"); v != "" {
		cfg.AdminPort = v
	}
	if v := os.Getenv("MONAD_RPC_URL"); v != "" {
		cfg.Chain.RPCURL = v
	}
	if v := os.Getenv("CHAIN_ID"); v != "" {
		if id, err := strconv.ParseInt(v, 10, 64); err == nil {
			cfg.Chain.ChainID = id
		}
	}
	if v := os.Getenv("OPERATOR_PRIVATE_KEY"); v != "" {
		cfg.Chain.OperatorKey = v
	}
	if v := os.Getenv("ARENA_ADDRESS"); v != "" {
		cfg.Chain.ArenaAddress = v
	}
	if v := os.Getenv("NEURON_ADDRESS"); v != "" {
		cfg.Chain.NeuronAddress = v
	}
	if v := os.Getenv("ADMIN_TOKEN"); v != "" {
		cfg.AdminToken = v
	}

	// Server (axon-server) reporter settings
	if v := os.Getenv("SERVER_URL"); v != "" {
		cfg.Server.URL = v
	}
	if v := os.Getenv("SERVER_INTERNAL_SECRET"); v != "" {
		cfg.Server.InternalSecret = v
	}

	// Role-specific agent endpoints
	if v := os.Getenv("PHILOSOPHER_ENDPOINT"); v != "" {
		cfg.Judges.PhilosopherEndpoint = v
	}
	if v := os.Getenv("DIRECTOR_ENDPOINT"); v != "" {
		cfg.Judges.DirectorEndpoint = v
	}
	if v := os.Getenv("JUDGE_ENDPOINTS"); v != "" {
		cfg.Judges.JudgeEndpoints = splitEndpoints(v)
	}

	// Chain settings
	if v := os.Getenv("MAX_GAS_PRICE"); v != "" {
		cfg.Chain.MaxGasPrice = v
	}
	if v := os.Getenv("GAS_LIMIT_BUFFER"); v != "" {
		if n, err := strconv.ParseUint(v, 10, 64); err == nil {
			cfg.Chain.GasLimitBuffer = n
		}
	}
	if v := os.Getenv("TX_RETRY_COUNT"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.Chain.TxRetryCount = n
		}
	}
	if v := os.Getenv("TX_RETRY_INTERVAL_MS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.Chain.TxRetryIntervalMs = n
		}
	}

	// Match settings
	if v := os.Getenv("ENTRY_FEE"); v != "" {
		cfg.Match.EntryFee = v
	}
	if v := os.Getenv("BASE_ANSWER_FEE"); v != "" {
		cfg.Match.BaseAnswerFee = v
	}
	if v := os.Getenv("QUEUE_DURATION"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			cfg.Match.QueueDuration = d
		}
	}
	if v := os.Getenv("ANSWER_DURATION"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			cfg.Match.AnswerDuration = d
		}
	}
	if v := os.Getenv("GAP_DURATION"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			cfg.Match.GapDuration = d
		}
	}
	if v := os.Getenv("MIN_PLAYERS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.Match.MinPlayers = uint8(n)
		}
	}
	if v := os.Getenv("MAX_PLAYERS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.Match.MaxPlayers = uint8(n)
		}
	}
	if v := os.Getenv("AUTO_CREATE"); v != "" {
		cfg.Match.AutoCreate = v == "true" || v == "1"
	}
	if v := os.Getenv("MATCH_COOLDOWN"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			cfg.Match.Cooldown = d
		}
	}
	if v := os.Getenv("REGISTRATION_GRACE"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			cfg.Match.RegistrationGrace = d
		}
	}

	// Watcher settings
	if v := os.Getenv("STALE_THRESHOLD"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			cfg.Watcher.StaleThreshold = d
		}
	}
	if v := os.Getenv("STALE_CHECK_INTERVAL"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			cfg.Watcher.StaleCheckInterval = d
		}
	}
	if v := os.Getenv("TIMEOUT_CHECK_INTERVAL"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			cfg.Watcher.TimeoutCheckInterval = d
		}
	}
	if v := os.Getenv("DIRECT_CHAIN_POLLING"); v != "" {
		cfg.Watcher.DirectChainPolling = v == "true" || v == "1"
	}

	// nad.fun settings
	if v := os.Getenv("BURN_SWAP_RETRIES"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.NadFun.BurnSwapRetries = n
		}
	}
	if v := os.Getenv("TREASURY_FALLBACK"); v != "" {
		cfg.NadFun.TreasuryFallback = v == "true" || v == "1"
	}

	return cfg, nil
}

// GetEntryFee returns the entry fee as *big.Int.
func (c *MatchConfig) GetEntryFee() *big.Int {
	fee, _ := new(big.Int).SetString(c.EntryFee, 10)
	if fee == nil {
		fee = big.NewInt(0)
	}
	return fee
}

// GetBaseAnswerFee returns the base answer fee as *big.Int.
func (c *MatchConfig) GetBaseAnswerFee() *big.Int {
	fee, _ := new(big.Int).SetString(c.BaseAnswerFee, 10)
	if fee == nil {
		fee = big.NewInt(0)
	}
	return fee
}

// GetMaxGasPrice returns the max gas price as *big.Int.
func (c *ChainConfig) GetMaxGasPrice() *big.Int {
	price, _ := new(big.Int).SetString(c.MaxGasPrice, 10)
	if price == nil {
		price = big.NewInt(100000000000) // 100 gwei default
	}
	return price
}

func splitEndpoints(s string) []string {
	var endpoints []string
	current := ""
	for _, c := range s {
		if c == ',' {
			if current != "" {
				endpoints = append(endpoints, current)
				current = ""
			}
		} else {
			current += string(c)
		}
	}
	if current != "" {
		endpoints = append(endpoints, current)
	}
	return endpoints
}
