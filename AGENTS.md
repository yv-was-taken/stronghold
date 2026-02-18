# Repository Guidelines

## Scope and Source of Truth
This file is the contributor guide for repository workflows. It is synchronized with `CLAUDE.md`. If this file and `CLAUDE.md` ever diverge, treat `CLAUDE.md` as authoritative and update this file immediately.

## Project Structure & Module Organization
- `cmd/`: binary entry points (`api`, `cli`, `proxy`, `x402test`).
- `internal/`: core Go modules (`handlers`, `middleware`, `db`, `wallet`, `proxy`, `cli`, `server`, `config`, `stronghold`).
- `internal/db/migrations/`: forward-only SQL migrations embedded into the API binary.
- `web/`: Next.js frontend with tests under `web/__tests__/`.
- `scripts/`: e2e and setup helpers.
- `facilitator/`: x402 settlement service deployment assets.

## Build, Test, and Development Commands
- Build:
  - `go build -o stronghold-api ./cmd/api`
  - `go build -o stronghold ./cmd/cli`
  - `go build -o stronghold-proxy ./cmd/proxy`
- Core tests:
  - `go test ./...`
  - `go test ./internal/handlers -run TestScanContent` (targeted run)
- Frontend (use Bun, not npm):
  - `cd web && bun run dev`
  - `cd web && bun run lint`
  - `cd web && bun run test`
- Local services:
  - `docker-compose up -d`
  - `docker-compose --profile with-proxy up -d`
  - `go run ./cmd/api`
  - `go run ./cmd/cli doctor`

## Coding Style & Naming Conventions
- Go: idiomatic structure, `gofmt` formatting, lowercase package names, `PascalCase` exports.
- TypeScript/React: `PascalCase` components and `useX` hooks.
- Wallet fields must be explicit per chain (`evm_wallet_address`, `solana_wallet_address`), not generic ambiguous names.

## Git Workflow (Mandatory)
- Never commit or push unless explicitly asked by the user in the current conversation state.
- Before committing changes that modify Go code (`*.go`, `go.mod`, `go.sum`), run `go test ./...` and verify all tests pass.
- Before committing changes that modify frontend TypeScript (`web/**/*.ts`, `web/**/*.tsx`), run `cd web && bun run test` and verify tests pass.
- Docs-only changes (for example `*.md`) do not require running Go or frontend tests.
- If Docker is required for tests, ensure Docker is running first; skipped tests are not equivalent to passing.
- Do not commit with failing tests.
- Verify changes locally before requesting/performing commit.
- For frontend changes, validate behavior locally with `cd web && bun run dev`.
- Do not push speculative commits.
- NEVER remove an active worktree directory. `git worktree prune` must only remove stale metadata. Before deleting any worktree directory, first verify it is not listed by `git worktree list` (or is confirmed orphaned/stale).

## Testing Guidelines (Mandatory)
- Do not use `-short`; run full suites.
- Be explicit when tests use mocks versus real external services.
- Never claim end-to-end correctness from mocked tests alone.
- x402 payment validity must be verified with real facilitator/e2e flows when payment protocol behavior is changed.
- If infrastructure is missing and tests are skipped, stop and resolve environment first.

## CLI UX and Documentation Sync
When changing any CLI user-facing behavior (commands, args, flags, output, wallet/account flows), update all of:
1. `cmd/cli/main.go` help text/examples.
2. `web/public/llms.txt`.
3. `web/public/llms-full.txt`.

## Implementation Discipline
- Fix root causes; do not add environment hacks/symlink workarounds for CLI issues.
- Confirm file and binary paths exist instead of assuming from directory layout.
- Do not guess implementations. Read docs/specs, inspect reference implementations and library source, then verify with real tests.
- Before planning, check current repository state (`git log` and relevant source files). Do not plan from stale assumptions.

## Temporary Files
- Do not create scratch/working files in the repository.
- Put temporary analysis and intermediate artifacts in `/tmp/` (or dedicated scratch locations), not project directories.

## Architecture and Runtime Notes
- API endpoints include `/health`, `/health/live`, `/health/ready`, `/v1/pricing`, `/v1/scan/content`, `/v1/scan/output`.
- Payment flow is x402-first; wallet signing is the payment mechanism.
- Stripe is only for wallet top-up (on-ramp), not an alternate request payment protocol.
- Keep wallet and test-wallet signing behavior aligned for x402/EIP-3009 changes.

## Database and Migrations
- Migrations are auto-run on API startup from `internal/db/migrations/`.
- Add migrations as `NNN_snake_case_description.sql`, forward-only.
- Each new migration must be validated with:
  - `go test ./internal/db/... -v`
  - `go test ./...`

## Deployment Guidelines
- API deploy: `fly deploy` from repo root.
- Facilitator deploy: `fly deploy` from `facilitator/`.
- Frontend deploys via Cloudflare Pages on push.
- Do not run local frontend production builds unless debugging a remote build failure.

# Agent Commit Policy

- Never create a git commit unless the user explicitly asks to commit in the current conversation state.
- Default behavior is to leave all code changes uncommitted for user review.
- Never push commits unless the user explicitly asks to push.
