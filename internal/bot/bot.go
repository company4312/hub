package bot

import (
	"context"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"syscall"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"github.com/company4312/copilot-telegram-bot/internal/agent"
	"github.com/company4312/copilot-telegram-bot/internal/store"
)

// Bot handles the Telegram update loop and dispatches messages to the agent.
type Bot struct {
	api             *tgbotapi.BotAPI
	pool            *agent.Pool
	authorizedUsers map[int64]bool
}

// New creates a Telegram bot instance with the given token and agent pool.
func New(token string, p *agent.Pool, authorizedUserIDs []int64) (*Bot, error) {
	api, err := tgbotapi.NewBotAPI(token)
	if err != nil {
		return nil, fmt.Errorf("telegram auth: %w", err)
	}
	log.Printf("Telegram bot authorized as @%s", api.Self.UserName)

	allowed := make(map[int64]bool, len(authorizedUserIDs))
	for _, id := range authorizedUserIDs {
		allowed[id] = true
	}
	if len(allowed) > 0 {
		log.Printf("Authorization enabled: %d allowed user(s)", len(allowed))
	} else {
		log.Println("WARNING: No AUTHORIZED_USERS configured — bot is open to everyone")
	}

	b := &Bot{api: api, pool: p, authorizedUsers: allowed}

	// Wire up the notify callback so the agent pool can send async messages.
	p.SetNotifyFunc(b.Reply)

	return b, nil
}

// Run starts the long-polling update loop. It blocks until ctx is cancelled.
func (b *Bot) Run(ctx context.Context) error {
	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60

	updates := b.api.GetUpdatesChan(u)

	for {
		select {
		case <-ctx.Done():
			b.api.StopReceivingUpdates()
			return nil
		case update, ok := <-updates:
			if !ok {
				return nil
			}
			if update.Message == nil || update.Message.Text == "" {
				continue
			}
			go b.handleMessage(ctx, update.Message)
		}
	}
}

func (b *Bot) handleMessage(ctx context.Context, msg *tgbotapi.Message) {
	chatID := msg.Chat.ID

	if !b.isAuthorized(msg.From) {
		log.Printf("unauthorized message from user %d (%s) in chat %d",
			msg.From.ID, msg.From.UserName, chatID)
		b.Reply(chatID, "⛔ You are not authorized to use this bot.")
		return
	}

	if msg.IsCommand() {
		b.handleCommand(ctx, msg)
		return
	}

	// Show "typing…" indicator while processing.
	action := tgbotapi.NewChatAction(chatID, tgbotapi.ChatTyping)
	_, _ = b.api.Send(action)

	_, response, err := b.pool.HandleUserMessage(ctx, chatID, msg.Text)
	if err != nil {
		log.Printf("agent error for chat %d: %v", chatID, err)
		b.Reply(chatID, "Sorry, something went wrong. Please try again.")
		return
	}

	if response == "" {
		response = "(empty response)"
	}

	b.Reply(chatID, response)
}

func (b *Bot) handleCommand(ctx context.Context, msg *tgbotapi.Message) {
	chatID := msg.Chat.ID

	switch msg.Command() {
	case "start":
		b.Reply(chatID,
			"👋 Hi! I'm a Copilot-powered assistant.\n\n"+
				"Send me any message and I'll respond using GitHub Copilot.\n\n"+
				"Commands:\n"+
				"/status — Show active task status\n"+
				"/thread — Manage conversation threads\n"+
				"/restart — Rebuild and reload the bot\n"+
				"/help — Show this message")
	case "status":
		summary, err := b.pool.GetTaskSummary()
		if err != nil {
			log.Printf("status error: %v", err)
			b.Reply(chatID, "Failed to get task status.")
			return
		}
		b.Reply(chatID, summary)
	case "restart":
		b.Reply(chatID, "♻️ Rebuilding and restarting…")
		ppid := os.Getppid()
		if err := syscall.Kill(ppid, syscall.SIGUSR1); err != nil {
			log.Printf("restart signal error: %v", err)
			b.Reply(chatID, fmt.Sprintf("Failed to signal supervisor (pid %d): %v", ppid, err))
		}
	case "help":
		b.Reply(chatID,
			"Available commands:\n"+
				"/start — Welcome message\n"+
				"/status — Show active task status\n"+
				"/thread — Manage conversation threads\n"+
				"/restart — Rebuild and reload the bot\n"+
				"/help — Show this message\n\n"+
				"Just send any text message to chat with the AI.")
	case "thread":
		b.handleThread(chatID, msg.CommandArguments())
	default:
		b.Reply(chatID, "Unknown command. Use /help to see available commands.")
	}
}

// Reply sends a message to a Telegram chat.
func (b *Bot) Reply(chatID int64, text string) {
	msg := tgbotapi.NewMessage(chatID, text)
	if _, err := b.api.Send(msg); err != nil {
		log.Printf("failed to send message to chat %d: %v", chatID, err)
	}
}

func (b *Bot) isAuthorized(user *tgbotapi.User) bool {
	if len(b.authorizedUsers) == 0 {
		return true
	}
	if user == nil {
		return false
	}
	return b.authorizedUsers[user.ID]
}

// ParseAuthorizedUsers parses a comma-separated string of Telegram user IDs.
func ParseAuthorizedUsers(raw string) []int64 {
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	var ids []int64
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		id, err := strconv.ParseInt(p, 10, 64)
		if err != nil {
			log.Printf("WARNING: invalid user ID %q in AUTHORIZED_USERS, skipping", p)
			continue
		}
		ids = append(ids, id)
	}
	return ids
}

func (b *Bot) handleThread(chatID int64, args string) {
	sub := strings.Fields(args)
	if len(sub) == 0 {
		b.Reply(chatID,
			"Thread commands:\n"+
				"/thread new — Start a new thread\n"+
				"/thread list — List recent threads\n"+
				"/thread switch <N> — Switch to thread #N from list")
		return
	}

	switch sub[0] {
	case "new":
		active, err := b.pool.GetActiveThread(chatID)
		if err != nil {
			log.Printf("thread new error: %v", err)
			b.Reply(chatID, "Failed to get active thread.")
			return
		}
		if active != nil {
			_ = b.pool.UpdateThreadStatus(active.ID, "archived")
			b.Reply(chatID, fmt.Sprintf("New thread started. Previous: %s", active.Title))
		} else {
			b.Reply(chatID, "New thread started.")
		}

	case "list":
		threads, err := b.pool.ListThreads(store.ThreadFilter{ChatID: chatID, Limit: 10})
		if err != nil {
			log.Printf("thread list error: %v", err)
			b.Reply(chatID, "Failed to list threads.")
			return
		}
		if len(threads) == 0 {
			b.Reply(chatID, "No threads yet.")
			return
		}
		var sb strings.Builder
		for i, t := range threads {
			status := ""
			if t.Status == "active" {
				status = " (active)"
			}
			fmt.Fprintf(&sb, "#%d%s %s — %s\n", i+1, status, t.Title, relativeTime(t.UpdatedAt))
		}
		b.Reply(chatID, sb.String())

	case "switch":
		if len(sub) < 2 {
			b.Reply(chatID, "Usage: /thread switch <N>")
			return
		}
		n, err := strconv.Atoi(sub[1])
		if err != nil || n < 1 {
			b.Reply(chatID, "Please provide a valid thread number.")
			return
		}
		threads, err := b.pool.ListThreads(store.ThreadFilter{ChatID: chatID, Limit: 10})
		if err != nil {
			log.Printf("thread switch error: %v", err)
			b.Reply(chatID, "Failed to list threads.")
			return
		}
		if n > len(threads) {
			b.Reply(chatID, fmt.Sprintf("Only %d threads available.", len(threads)))
			return
		}
		target := threads[n-1]

		// Archive the currently active thread.
		active, _ := b.pool.GetActiveThread(chatID)
		if active != nil && active.ID != target.ID {
			_ = b.pool.UpdateThreadStatus(active.ID, "archived")
		}

		// Activate the selected thread.
		if err := b.pool.UpdateThreadStatus(target.ID, "active"); err != nil {
			log.Printf("thread switch error: %v", err)
			b.Reply(chatID, "Failed to switch thread.")
			return
		}
		b.Reply(chatID, fmt.Sprintf("Switched to: %s", target.Title))

	default:
		b.Reply(chatID,
			"Thread commands:\n"+
				"/thread new — Start a new thread\n"+
				"/thread list — List recent threads\n"+
				"/thread switch <N> — Switch to thread #N from list")
	}
}

// relativeTime returns a human-readable relative time string.
func relativeTime(rfc3339 string) string {
	t, err := time.Parse(time.RFC3339, rfc3339)
	if err != nil {
		return rfc3339
	}
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		m := int(d.Minutes())
		if m == 1 {
			return "1 min ago"
		}
		return fmt.Sprintf("%d mins ago", m)
	case d < 24*time.Hour:
		h := int(d.Hours())
		if h == 1 {
			return "1 hour ago"
		}
		return fmt.Sprintf("%d hours ago", h)
	default:
		days := int(d.Hours() / 24)
		if days == 1 {
			return "1 day ago"
		}
		return fmt.Sprintf("%d days ago", days)
	}
}
