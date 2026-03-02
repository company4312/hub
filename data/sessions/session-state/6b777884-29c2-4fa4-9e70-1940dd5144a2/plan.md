# Multi-Agent Architecture

## Problem
The bot currently has a single hardcoded CTO agent. We need to refactor so the CTO is just one agent in a registry, and the system supports multiple agents with their own sessions, system prompts, and models. This sets us up for hiring more employees later and a future `/switch` command.

## Approach
Refactor the agent layer into a **Pool** that manages multiple named agents. Each agent has its own config (system prompt, model, title) and maintains separate Copilot sessions per chat. The store gets an agent registry table and per-agent session tracking. The CTO becomes the first registered agent — no longer special-cased.

## Todos

1. **store-schema** — Add `agents` table and `chat_agents` table to store. Update `sessions` table to include `agent_name` column. Add migration logic.
2. **store-methods** — Add store methods: RegisterAgent, ListAgents, GetAgent, GetActiveAgent, SetActiveAgent, plus update session methods to be agent-aware.
3. **agent-refactor** — Refactor `internal/agent/` from single-agent `Agent` to multi-agent `Pool`. Each agent config lives in the pool, sessions are keyed by (agent_name, chat_id).
4. **bot-update** — Update bot to use the new Pool interface. Minimal changes — same external behavior for now.
5. **seed-cto** — Register the CTO agent on startup with its system prompt and model.
6. **build-verify** — Build and verify everything compiles cleanly.

## Notes
- Single `copilot.Client` shared across all agents (it's just the CLI server process)
- Sessions are per (agent, chat) — different agents in the same chat have separate sessions
- Default active agent per chat = "cto"
- Future: `/switch` command changes active agent, `/hire` creates new agents
