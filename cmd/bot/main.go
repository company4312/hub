package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/company4312/copilot-telegram-bot/internal/agent"
	"github.com/company4312/copilot-telegram-bot/internal/api"
	"github.com/company4312/copilot-telegram-bot/internal/bot"
	"github.com/company4312/copilot-telegram-bot/internal/store"
)

func main() {
	token := os.Getenv("TELEGRAM_BOT_TOKEN")
	if token == "" {
		log.Fatal("TELEGRAM_BOT_TOKEN environment variable is required")
	}

	// Ensure the data directory exists.
	dataDir := "data"
	if err := os.MkdirAll(dataDir, 0o750); err != nil {
		log.Fatalf("create data dir: %v", err)
	}

	// Initialize the session store.
	dbPath := filepath.Join(dataDir, "bot.db")
	s, err := store.New(dbPath)
	if err != nil {
		log.Fatalf("open database: %v", err)
	}
	defer func() { _ = s.Close() }()

	// Initialize and start the agent pool.
	configDir := filepath.Join(dataDir, "sessions")
	if err := os.MkdirAll(configDir, 0o750); err != nil {
		log.Fatalf("create config dir: %v", err)
	}

	// Load and register agents from YAML configs.
	agentsDir := filepath.Join(dataDir, "agents")
	agents, err := store.LoadAgentConfigs(agentsDir)
	if err != nil {
		log.Fatalf("load agent configs: %v", err)
	}
	for _, cfg := range agents {
		if err := s.RegisterAgent(cfg); err != nil {
			log.Fatalf("register agent %s: %v", cfg.Name, err)
		}
	}
	log.Printf("Registered %d agents", len(agents))

	p := agent.NewPool(s, configDir)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := p.Start(ctx); err != nil {
		log.Fatalf("start agent pool: %v", err)
	}
	defer func() { _ = p.Stop() }()

	// Start the dashboard API server.
	dashPort := os.Getenv("DASHBOARD_PORT")
	if dashPort == "" {
		dashPort = "8080"
	}
	apiSrv := api.New(s, p, ":"+dashPort)
	if err := apiSrv.Start(); err != nil {
		log.Fatalf("start dashboard api: %v", err)
	}
	defer func() { _ = apiSrv.Stop(context.Background()) }()
	p.SetAPIServer(apiSrv)

	// Initialize the Telegram bot.
	authorizedUsers := bot.ParseAuthorizedUsers(os.Getenv("AUTHORIZED_USERS"))
	b, err := bot.New(token, p, authorizedUsers)
	if err != nil {
		log.Fatalf("init telegram bot: %v", err)
	}

	// Graceful shutdown on SIGINT/SIGTERM.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		sig := <-sigCh
		log.Printf("received %s, shutting down…", sig)
		cancel()
	}()

	log.Println("Bot is running. Press Ctrl+C to stop.")
	if err := b.Run(ctx); err != nil {
		log.Fatalf("bot error: %v", err)
	}
	log.Println("Shutdown complete.")
}
