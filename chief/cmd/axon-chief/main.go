// Package main is the entry point for the axon-chief service.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"

	"github.com/axon-arena/axon-chief/internal/chain"
	"github.com/axon-arena/axon-chief/internal/config"
	"github.com/axon-arena/axon-chief/internal/judge"
	"github.com/axon-arena/axon-chief/internal/match"
	"github.com/axon-arena/axon-chief/internal/server"
)

var (
	configPath = flag.String("config", "", "Path to config file")
	version    = "1.0.0"
)

func main() {
	_ = godotenv.Load() // .env optional

	flag.Parse()

	log.SetFlags(log.LstdFlags | log.Lshortfile)
	log.Printf("Starting axon-chief %s", version)

	// Load configuration
	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// Validate required settings
	if cfg.Chain.OperatorKey == "" {
		log.Fatal("OPERATOR_PRIVATE_KEY environment variable is required")
	}
	if cfg.Chain.ArenaAddress == "" {
		log.Fatal("ARENA_ADDRESS environment variable is required")
	}
	if cfg.Chain.NeuronAddress == "" {
		log.Fatal("NEURON_ADDRESS environment variable is required")
	}

	// Create chain client
	log.Printf("Connecting to Monad RPC: %s (chainId: %d)", cfg.Chain.RPCURL, cfg.Chain.ChainID)
	chainClient, err := chain.NewChainClient(&cfg.Chain)
	if err != nil {
		log.Fatalf("Failed to create chain client: %v", err)
	}
	defer chainClient.Close()

	log.Printf("Operator address: %s", chainClient.Address().Hex())

	// Verify operator is authorized
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	isOp, err := chainClient.IsOperator(ctx)
	cancel()
	if err != nil {
		log.Printf("Warning: failed to verify operator status: %v", err)
	} else if !isOp {
		log.Printf("Warning: operator %s is not authorized in AxonArena contract", chainClient.Address().Hex())
	} else {
		log.Printf("Operator is authorized in AxonArena contract")
	}

	// Get operator balance
	ctx, cancel = context.WithTimeout(context.Background(), 10*time.Second)
	balance, err := chainClient.GetBalance(ctx)
	cancel()
	if err != nil {
		log.Printf("Warning: failed to get operator balance: %v", err)
	} else {
		log.Printf("Operator balance: %s wei", balance.String())
	}

	// Create judge coordinator with role-specific endpoints
	log.Printf("Configuring agents: philosopher=%s, director=%s, judges=%d",
		cfg.Judges.PhilosopherEndpoint, cfg.Judges.DirectorEndpoint, len(cfg.Judges.JudgeEndpoints))
	judgeCoordinator := judge.NewCoordinator(&cfg.Judges)

	// Check agent health
	ctx, cancel = context.WithTimeout(context.Background(), 30*time.Second)
	statuses := judgeCoordinator.CheckHealth(ctx)
	cancel()
	healthyCount := 0
	for _, s := range statuses {
		if s.Healthy {
			healthyCount++
			log.Printf("%s %s: healthy (provider: %s)", s.Role, s.Endpoint, s.Provider)
		} else {
			log.Printf("%s %s: unhealthy (%v)", s.Role, s.Endpoint, s.Error)
		}
	}
	totalAgents := 2 + len(cfg.Judges.JudgeEndpoints) // philosopher + director + judges
	if healthyCount < totalAgents {
		log.Printf("Warning: only %d/%d agents healthy", healthyCount, totalAgents)
	}

	// Create match manager
	matchCfg := &match.Config{
		EntryFee:          cfg.Match.GetEntryFee(),
		BaseAnswerFee:     cfg.Match.GetBaseAnswerFee(),
		QueueDuration:     cfg.Match.QueueDuration,
		AnswerDuration:    cfg.Match.AnswerDuration,
		MinPlayers:        cfg.Match.MinPlayers,
		MaxPlayers:        cfg.Match.MaxPlayers,
		AutoCreate:        cfg.Match.AutoCreate,
		Cooldown:          cfg.Match.Cooldown,
		RegistrationGrace: cfg.Match.RegistrationGrace,
	}
	matchManager := match.NewManager(matchCfg)

	// Create server
	srv := server.NewServer(cfg, chainClient, judgeCoordinator, matchManager)

	// Setup gin router
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(gin.LoggerWithFormatter(func(param gin.LogFormatterParams) string {
		return fmt.Sprintf("[%s] %s %s %d %s\n",
			param.TimeStamp.Format("2006-01-02 15:04:05"),
			param.Method,
			param.Path,
			param.StatusCode,
			param.Latency,
		)
	}))

	// Setup routes
	srv.SetupRoutes(r)

	// Start background services
	ctx, cancel = context.WithCancel(context.Background())
	defer cancel()

	if err := srv.Start(ctx); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}

	// Create HTTP server (main API on cfg.Port)
	httpServer := &http.Server{
		Addr:    ":" + cfg.Port,
		Handler: r,
	}

	// Start server in goroutine
	go func() {
		log.Printf("Listening on :%s", cfg.Port)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Failed to start server: %v", err)
		}
	}()

	// Start admin server on AdminPort if different from main port
	var adminServer *http.Server
	if cfg.AdminPort != "" && cfg.AdminPort != cfg.Port {
		adminRouter := gin.New()
		adminRouter.Use(gin.Recovery())
		adminRouter.GET("/health", func(c *gin.Context) {
			c.JSON(200, gin.H{"status": "ok"})
		})
		adminRouter.GET("/status", func(c *gin.Context) {
			// Proxy to the main server's status handler
			r.ServeHTTP(c.Writer, c.Request)
		})

		adminServer = &http.Server{
			Addr:    ":" + cfg.AdminPort,
			Handler: adminRouter,
		}
		go func() {
			log.Printf("Admin server listening on :%s", cfg.AdminPort)
			if err := adminServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				log.Printf("Admin server error: %v", err)
			}
		}()
	}

	// Wait for interrupt signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("Shutting down...")

	// Stop background services
	srv.Stop()

	// Shutdown HTTP servers
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		log.Printf("Server shutdown error: %v", err)
	}

	if adminServer != nil {
		if err := adminServer.Shutdown(shutdownCtx); err != nil {
			log.Printf("Admin server shutdown error: %v", err)
		}
	}

	log.Println("Server stopped")
}
