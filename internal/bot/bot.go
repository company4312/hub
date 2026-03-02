package bot

import (
	"context"
	"fmt"
	"log"
	"os"
	"syscall"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"github.com/company4312/copilot-telegram-bot/internal/agent"
)

// Bot handles the Telegram update loop and dispatches messages to the agent.
type Bot struct {
	api  *tgbotapi.BotAPI
	pool *agent.Pool
}

// New creates a Telegram bot instance with the given token and agent pool.
func New(token string, p *agent.Pool) (*Bot, error) {
	api, err := tgbotapi.NewBotAPI(token)
	if err != nil {
		return nil, fmt.Errorf("telegram auth: %w", err)
	}
	log.Printf("Telegram bot authorized as @%s", api.Self.UserName)
	return &Bot{api: api, pool: p}, nil
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

	if msg.IsCommand() {
		b.handleCommand(ctx, msg)
		return
	}

	// Show "typing…" indicator while processing.
	action := tgbotapi.NewChatAction(chatID, tgbotapi.ChatTyping)
	b.api.Send(action)

	response, err := b.pool.SendMessage(ctx, chatID, msg.Text)
	if err != nil {
		log.Printf("agent error for chat %d: %v", chatID, err)
		b.reply(chatID, "Sorry, something went wrong. Please try again.")
		return
	}

	if response == "" {
		response = "(empty response)"
	}

	b.reply(chatID, response)
}

func (b *Bot) handleCommand(ctx context.Context, msg *tgbotapi.Message) {
	chatID := msg.Chat.ID

	switch msg.Command() {
	case "start":
		b.reply(chatID,
			"👋 Hi! I'm a Copilot-powered assistant.\n\n"+
				"Send me any message and I'll respond using GitHub Copilot.\n\n"+
				"Commands:\n"+
				"/reset — Clear conversation history\n"+
				"/restart — Rebuild and reload the bot\n"+
				"/help — Show this message")
	case "reset":
		if err := b.pool.ResetSession(ctx, chatID); err != nil {
			log.Printf("reset error for chat %d: %v", chatID, err)
			b.reply(chatID, "Failed to reset session. Please try again.")
			return
		}
		b.reply(chatID, "Conversation cleared. Send a new message to start fresh.")
	case "restart":
		b.reply(chatID, "♻️ Rebuilding and restarting…")
		ppid := os.Getppid()
		if err := syscall.Kill(ppid, syscall.SIGUSR1); err != nil {
			log.Printf("restart signal error: %v", err)
			b.reply(chatID, fmt.Sprintf("Failed to signal supervisor (pid %d): %v", ppid, err))
		}
	case "help":
		b.reply(chatID,
			"Available commands:\n"+
				"/start — Welcome message\n"+
				"/reset — Clear conversation history\n"+
				"/restart — Rebuild and reload the bot\n"+
				"/help — Show this message\n\n"+
				"Just send any text message to chat with the AI.")
	default:
		b.reply(chatID, "Unknown command. Use /help to see available commands.")
	}
}

func (b *Bot) reply(chatID int64, text string) {
	msg := tgbotapi.NewMessage(chatID, text)
	if _, err := b.api.Send(msg); err != nil {
		log.Printf("failed to send message to chat %d: %v", chatID, err)
	}
}
