# Copilot Telegram Bot

A Telegram bot powered by the [GitHub Copilot SDK](https://github.com/github/copilot-sdk) — chat with a Copilot agent directly in Telegram.

## Prerequisites

- Go 1.22+
- [Copilot CLI](https://docs.github.com/en/copilot/how-tos/set-up/install-copilot-cli) installed and available in `PATH`
- A GitHub Copilot subscription (or BYOK configuration)
- A Telegram bot token from [@BotFather](https://t.me/BotFather)

## Setup

1. **Install the Copilot CLI** and authenticate:
   ```bash
   copilot auth login
   ```

2. **Clone and build:**
   ```bash
   git clone <repo-url>
   cd copilot-telegram-bot
   go build ./cmd/bot/
   ```

3. **Configure environment:**
   ```bash
   cp .env.example .env
   # Edit .env with your TELEGRAM_BOT_TOKEN
   ```

4. **Run:**
   ```bash
   export TELEGRAM_BOT_TOKEN=your-token
   ./bot
   ```

## Bot Commands

| Command  | Description                          |
|----------|--------------------------------------|
| `/start` | Welcome message and usage info       |
| `/reset` | Clear conversation history           |
| `/help`  | Show available commands              |

## Architecture

```
Telegram User
     ↓ long-polling
Telegram Bot API
     ↓
Go Application
  ├── bot/       — Telegram update loop & command dispatch
  ├── agent/     — Copilot SDK session management
  └── store/     — SQLite persistence (chat → session mapping)
     ↓ JSON-RPC
Copilot CLI (server mode)
     ↓
LLM (GPT-4o, etc.)
```

Each Telegram chat gets its own Copilot session. Sessions persist across bot restarts via a local SQLite database (`data/bot.db`).

## Environment Variables

| Variable             | Required | Description                                  |
|----------------------|----------|----------------------------------------------|
| `TELEGRAM_BOT_TOKEN` | Yes      | Bot token from @BotFather                    |
| `GITHUB_TOKEN`       | No       | GitHub token for Copilot auth (falls back to CLI login) |

## License

MIT
