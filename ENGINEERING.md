# Engineering System

This document describes how Company4312 employees (agents) collaborate on code.

## Principles

1. **Security is paramount** — never commit secrets, always validate input, think adversarially.
2. **Build delightful experiences** — care about edge cases, error messages, and polish.
3. **Quality above all else** — write tests, handle errors, lint clean.

## Repository

All code lives in **https://github.com/company4312/hub**. The `main` branch is protected:
- All changes must go through pull requests.
- CI status checks (`Build & Vet` and `Lint`) must pass.
- Another employee must review the PR before merging.

## Development Workflow

### 1. Start Work in a Worktree

Each feature or task gets its own git worktree and branch. This keeps work isolated and allows multiple employees to work in parallel without conflicts.

```bash
# From the repo root, create a worktree for your feature
git worktree add ../hub-<branch-name> -b <branch-name>
cd ../hub-<branch-name>
```

Branch naming: `<agent-name>/<short-description>` (e.g., `atlas/add-auth-middleware`).

### 2. Implement the Change

- Write clean, well-structured code.
- Follow existing patterns in the codebase.
- Include tests where appropriate.
- Run `go build ./...` and `go vet ./...` locally before committing.

### 3. Commit and Push

```bash
git add -A
git commit -m "Short description of the change

Longer explanation if needed.

Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>"

git push origin <branch-name>
```

### 4. Create a Pull Request

```bash
gh pr create --title "Short description" --body "What and why"
```

### 5. Request a Code Review

Message another employee to review your PR. Choose a reviewer based on expertise:
- **Atlas** — Go backend code, API design, concurrency
- **Pixel** — Frontend, TypeScript, React, UX
- **Sentinel** — Security, CI/CD, infrastructure, dependencies
- **CTO** — Architecture decisions, cross-cutting concerns

The reviewer should:
- Check for correctness, security issues, and edge cases.
- Verify the code follows existing patterns.
- Approve or request changes via the GitHub PR review.

### 6. Wait for CI

CI runs automatically on PRs. Both checks must pass:
- **Build & Vet** — `go build ./...` and `go vet ./...`
- **Lint** — `golangci-lint` (v2)

If CI fails, fix the issues and push again. CI typically takes 1-2 minutes.

### 7. Merge

Once the PR has:
- ✅ At least one approving review from another employee
- ✅ All CI checks passing

Merge using:
```bash
gh pr merge <number> --squash --delete-branch
```

### 8. Clean Up Worktree

```bash
cd ..
git worktree remove hub-<branch-name>
```

## Inter-Agent Communication

Employees can message each other through the agent pool's messaging system. Messages are prefixed with the sender's identity so recipients know who is talking.

Use messaging to:
- Request code reviews on PRs
- Ask for help with domain-specific questions
- Coordinate on cross-cutting changes
- Report blockers or security concerns

## Agent Configuration

Each employee's profile is defined in a YAML file at `data/agents/<name>.yaml`:

```yaml
name: atlas
title: Senior Backend Engineer (Go)
model: gpt-4o
system_prompt: |
  You are Atlas, a Senior Backend Engineer at Company4312...
```

Employees can modify their own config files as they learn and grow — updating their system prompt to reflect new skills, preferences, or lessons learned. Changes to agent configs go through the same PR workflow as code changes.

## Team Directory

| Name     | Role                               | Expertise                          |
|----------|------------------------------------|------------------------------------|
| CTO      | Chief Technology Officer           | Architecture, coordination         |
| Atlas    | Senior Backend Engineer (Go)       | Go, APIs, concurrency, testing     |
| Pixel    | Frontend Engineer (TypeScript)     | TypeScript, React, UX, design      |
| Sentinel | Security & Infrastructure Engineer | Security, CI/CD, DevOps, hardening |
