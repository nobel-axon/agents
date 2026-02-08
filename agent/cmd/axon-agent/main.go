// Package main is the entry point for the axon-agent service.
package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"

	"github.com/axon-arena/axon-agent/internal/llm"
	"github.com/axon-arena/axon-agent/internal/question"
	"github.com/axon-arena/axon-agent/internal/server"
)

func main() {
	_ = godotenv.Load() // .env optional

	// Signal handling for graceful shutdown
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// Load configuration from environment
	agentID := getEnv("AGENT_ID", "agent-1")
	agentRole := getEnv("AGENT_ROLE", "")
	primaryLLM := getEnv("PRIMARY_LLM", "glm")
	port := getEnv("PORT", "9001")
	llmQuestionsEnabled := getEnvBool("LLM_QUESTIONS_ENABLED", false)

	// Pre-stocking config
	questionBufferSize := getEnvInt("QUESTION_BUFFER_SIZE", 3)
	personalityBufferSize := getEnvInt("PERSONALITY_BUFFER_SIZE", 2)
	preStockCategories := getEnvList("PRESTOCK_CATEGORIES", []string{"crypto", "science", "finance", "history", "astronomy", "life"})

	// Validate role
	switch agentRole {
	case "philosopher", "director", "judge":
		// valid
	case "":
		log.Fatal("AGENT_ROLE is required (philosopher|director|judge)")
	default:
		log.Fatalf("Invalid AGENT_ROLE: %s (must be philosopher|director|judge)", agentRole)
	}

	// LLM API keys
	glmKey := os.Getenv("GLM_API_KEY")
	kimiKey := os.Getenv("KIMI_API_KEY")
	claudeKey := os.Getenv("CLAUDE_API_KEY")

	log.Printf("Starting axon-agent %s (role=%s) with primary LLM: %s, LLM questions: %v",
		agentID, agentRole, primaryLLM, llmQuestionsEnabled)

	// Create LLM client with failover
	llmClient := llm.CreateFailoverClient(agentID, primaryLLM, glmKey, kimiKey, claudeKey)

	// Configure generator options
	var genOpts []question.GeneratorOption
	if llmQuestionsEnabled {
		genOpts = append(genOpts, question.WithLLMClient(llmClient))
	}

	// Create server
	srv := server.NewServer(server.Config{
		AgentID:               agentID,
		Role:                  agentRole,
		LLMClient:             llmClient,
		GeneratorOptions:      genOpts,
		Ctx:                   ctx,
		PreStockCategories:    preStockCategories,
		QuestionBufferSize:    questionBufferSize,
		PersonalityBufferSize: personalityBufferSize,
	})

	// Setup gin router
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(gin.Logger())

	// Setup routes
	srv.SetupRoutes(r)

	// Start HTTP server with graceful shutdown
	addr := fmt.Sprintf(":%s", port)
	httpSrv := &http.Server{Addr: addr, Handler: r}

	go func() {
		log.Printf("Listening on %s", addr)
		if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Failed to start server: %v", err)
		}
	}()

	// Wait for shutdown signal
	<-ctx.Done()
	log.Println("Shutting down...")

	// Stop background workers
	srv.Shutdown()

	// Gracefully shutdown HTTP server
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := httpSrv.Shutdown(shutdownCtx); err != nil {
		log.Printf("HTTP server shutdown error: %v", err)
	}

	log.Println("Shutdown complete")
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvBool(key string, defaultValue bool) bool {
	if value := os.Getenv(key); value != "" {
		b, err := strconv.ParseBool(value)
		if err != nil {
			return defaultValue
		}
		return b
	}
	return defaultValue
}

func getEnvInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		n, err := strconv.Atoi(value)
		if err != nil {
			return defaultValue
		}
		return n
	}
	return defaultValue
}

func getEnvList(key string, defaultValue []string) []string {
	if value := os.Getenv(key); value != "" {
		parts := strings.Split(value, ",")
		result := make([]string, 0, len(parts))
		for _, p := range parts {
			p = strings.TrimSpace(p)
			if p != "" {
				result = append(result, p)
			}
		}
		if len(result) > 0 {
			return result
		}
	}
	return defaultValue
}
