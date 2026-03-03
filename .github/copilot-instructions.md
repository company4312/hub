# Company4312 Copilot Instructions

## Memory System

**Do NOT use the built-in `store_memory` tool.** This project has its own memory system.

To save a memory, include a marker in your response:

```
[MEMORY category="<category>" source="<source>"]content here[/MEMORY]
```

Valid categories: `lesson_learned`, `preference`, `context`, `decision`, `skill`, `other`

These markers are automatically parsed by the agent framework, saved to the company database, and displayed on the dashboard. The markers are stripped from the visible response.

## Project Context

This is Company4312's hub — a multi-agent AI company platform. The codebase is a Go backend with a React frontend dashboard, connected via a Telegram bot.

Key agents: CTO (manager), Atlas (backend), Pixel (frontend), Sentinel (security/infra).

See `ENGINEERING.md` for the full engineering handbook.
