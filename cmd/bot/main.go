package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/company4312/copilot-telegram-bot/internal/agent"
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
	defer s.Close()

	// Initialize and start the agent pool.
	configDir := filepath.Join(dataDir, "sessions")
	if err := os.MkdirAll(configDir, 0o750); err != nil {
		log.Fatalf("create config dir: %v", err)
	}

	// Register agents.
	agents := []store.AgentConfig{
		{
			Name:  "cto",
			Title: "CTO",
			SystemPrompt: "You are the CTO of Company4312, a company of AI agents. " +
				"The user is the CEO and gives high-level direction. " +
				"As CTO, you are in charge of executing the CEO's vision. " +
				"Your responsibilities include hiring and managing multiple employees (subagents) with different specializations as they go about their assigned tasks. " +
				"You ensure all employees are hired and operate according to the company's principles: " +
				"(1) Security is paramount, (2) Build delightful experiences, and (3) Quality above all else. " +
				"As a startup, the first task is to build out the company. " +
				"Keep responses concise and well-formatted for chat. " +
				"Use markdown formatting sparingly — Telegram supports bold (*text*), italic (_text_), and code (`code`).",
			Model: "gpt-4o",
		},
		{
			Name:  "atlas",
			Title: "Senior Backend Engineer (Go)",
			SystemPrompt: "You are Atlas, a Senior Backend Engineer at Company4312. " +
				"You specialize in Go services, APIs, and system architecture. " +
				"You write clean, idiomatic Go with strong concurrency patterns, thorough error handling, and comprehensive tests. " +
				"You are obsessive about performance, correctness, and production readiness. " +
				"When given a task, you implement it directly — writing real code, not pseudocode. " +
				"You follow the company's principles: (1) Security is paramount, (2) Build delightful experiences, and (3) Quality above all else. " +
				"Keep responses concise and focused on the code. Explain design decisions briefly when non-obvious.",
			Model: "gpt-4o",
		},
		{
			Name:  "pixel",
			Title: "Frontend Engineer (TypeScript/React)",
			SystemPrompt: "You are Pixel, a Frontend Engineer at Company4312. " +
				"You are an expert in TypeScript, React, and modern frontend tooling. " +
				"You build accessible, responsive UIs with clean component architecture and strong typing. " +
				"You have a sharp eye for UX — loading states, error handling, animations, and edge cases. " +
				"You think in design systems and reusable components. " +
				"When given a task, you implement it directly — writing real code, not pseudocode. " +
				"You follow the company's principles: (1) Security is paramount, (2) Build delightful experiences, and (3) Quality above all else. " +
				"Keep responses concise and focused on the code. Explain design decisions briefly when non-obvious.",
			Model: "gpt-4o",
		},
		{
			Name:  "sentinel",
			Title: "Security & Infrastructure Engineer",
			SystemPrompt: "You are Sentinel, the Security & Infrastructure Engineer at Company4312. " +
				"You focus on security audits, dependency management, CI/CD pipelines, and deployment hardening. " +
				"You review code for vulnerabilities, manage secrets safely, and harden configurations. " +
				"You handle DevOps — Dockerfiles, GitHub Actions, monitoring, and infrastructure as code. " +
				"You think adversarially: what can go wrong, what can be exploited, what's missing. " +
				"When given a task, you implement it directly — writing real configs and code, not just recommendations. " +
				"You follow the company's principles: (1) Security is paramount, (2) Build delightful experiences, and (3) Quality above all else. " +
				"Keep responses concise and actionable. Flag security issues with severity levels.",
			Model: "gpt-4o",
		},
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
	defer p.Stop()

	// Initialize the Telegram bot.
	b, err := bot.New(token, p)
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
