# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Git Workflow

**CRITICAL: Do NOT commit or push unless explicitly told to do so.**

- **ALWAYS run `go test ./...` and verify ALL tests pass before committing.** No exceptions. If Docker is not running, start it first. If tests fail, fix them before committing. Never commit with failing tests.
- Make changes and verify they work locally first
- Wait for explicit user approval before committing
- Never assume a change is ready to commit just because you made it
- Always test frontend changes locally with `bun run dev` before considering them complete
- Do not push multiple speculative commits hoping one works

## Project Overview

Stronghold is a pay-per-request AI security scanning platform with three components:
1. **API Server** (`cmd/api/`) - Go HTTP service using Fiber v3 with x402 crypto payment integration
2. **CLI Client** (`cmd/cli/`) - Cobra/Bubbletea-based CLI for system setup and account management
3. **Transparent Proxy** (`cmd/proxy/`) - Network-level traffic interceptor that scans all HTTP/HTTPS traffic

The platform provides 4-layer security scanning: heuristics, ML classification (Citadel/Hugot), semantic similarity, and optional LLM classification.

## Temporary Files

**IMPORTANT: Never create temporary or working files in the project directory.**

- Review documents, scratch notes, analysis outputs → `/tmp/` or the scratchpad directory
- Only production code, tests, and essential documentation belong in the repo
- If you need to write intermediate results, use `/tmp/` not the project root

## Build Commands

```bash
# Build all binaries
go build -o stronghold-api ./cmd/api
go build -o stronghold ./cmd/cli
go build -o stronghold-proxy ./cmd/proxy

# Run tests
go test ./...

# Run a specific test
go test ./internal/handlers -run TestScanContent

# Frontend (in web/ directory) - USE BUN, NOT NPM
cd web && bun run dev     # Development server
cd web && bun run build   # Build for production
cd web && bun run lint    # Lint TypeScript/React
cd web && bun run test    # Run tests
```

## Development Environment

```bash
# Start PostgreSQL and API with Docker Compose
docker-compose up -d

# With Caddy reverse proxy
docker-compose --profile with-proxy up -d

# Run API locally (requires DB)
go run ./cmd/api

# Run CLI locally
go run ./cmd/cli doctor
```

Required environment variables for local development (see `.env.example`):
- `DB_HOST`, `DB_PORT`, `DB_USER`, `DB_PASSWORD`, `DB_NAME` - PostgreSQL connection
- `JWT_SECRET` - Authentication token signing
- `X402_WALLET_ADDRESS` - Payment receiving address (omit for dev mode without payments)

## CLI Development

**Fix bugs properly - no workarounds.**

- When CLI code has bugs (e.g., path resolution issues), fix the actual code rather than adding workarounds like symlinks or environment hacks
- CLI must work correctly in all environments: local development, installed via release, and Docker containers
- Always verify file/binary existence before returning paths - don't assume files exist based on directory structure alone

## Research Before Implementation - NO GUESSING

**CRITICAL: Never guess at implementations. Always research first. This applies to EVERYTHING.**

For ANY implementation, you MUST:
1. **Read the documentation** - Find and read the actual docs, specs, or API references
2. **Find reference implementations** - Look at working code examples in the same language
3. **Check library source code** - When using libraries, read the source to understand expected types and behavior
4. **Verify with real tests** - Don't assume mock-based tests prove correctness

**DO NOT:**
- Guess at data formats, types, or behavior and "try things"
- Assume something works because it compiles
- Trust that mocked tests prove correctness
- Make changes without understanding what you're changing
- Say something "works" without actually verifying it end-to-end

This applies to EVERYTHING: APIs, libraries, protocols, file formats, CLI tools, database queries, etc. If you don't know how something works, research it first. Do not iterate by trial and error unless the user explicitly asks you to.

## Always Check Current State Before Planning

**CRITICAL: Before writing ANY plan, ALWAYS check the current state of the codebase first.**

- Read `git log` to see recent commits - work may already be done
- If `git log` doesn't yield enough insight, read the actual source files and check relevant parts of the codebase for more information
- Never write a plan based on an outdated understanding from a previous conversation or summary
- Context summaries from prior sessions may be stale - the codebase is the source of truth
- If resuming a conversation, assume the codebase may have changed since the last session
- **NEVER assume - always verify.** If you're unsure whether something exists or has been done, check the code. When possible, confirm before acting.

**This is non-negotiable.** Writing a plan for already-completed work wastes time and erodes trust.

## Architecture

```
internal/
├── server/          # Fiber HTTP server setup
├── handlers/        # API endpoint handlers (scan.go, auth.go, health.go, pricing.go)
├── middleware/      # x402 payment verification middleware
├── config/          # Environment configuration loading
├── stronghold/      # Citadel ML scanner integration
├── wallet/          # OS keyring operations and x402 payment signing
├── cli/             # CLI commands (doctor, install, enable/disable, status)
├── proxy/           # Transparent proxy server and API client
└── db/              # PostgreSQL database layer with migrations
```

**Key patterns:**
- Handlers receive `fiber.Ctx` and return JSON responses
- Payment middleware checks `X-PAYMENT` header and returns 402 if invalid
- CLI uses iptables/nftables (Linux) or pf (macOS) for transparent proxying
- Wallet credentials stored in OS-native keyring (macOS Keychain, Linux Secret Service/KWallet/pass)

## API Endpoints

- `/health`, `/health/live`, `/health/ready` - Health checks (no auth)
- `/v1/pricing` - Endpoint pricing (no auth)
- `/v1/scan/content` - Prompt injection detection ($0.001)
- `/v1/scan/output` - Credential leak detection ($0.001)

## Payment Architecture

**IMPORTANT: All payments use x402 protocol exclusively.**

- Every API request requiring payment uses x402 with the embedded wallet
- The wallet is stored in the OS keyring and signs payment headers automatically
- Stripe integration is **only** for wallet top-up (crypto on-ramp) - NOT a separate payment method
- Users fund their wallet via:
  1. Stripe on-ramp through the dashboard (converts fiat → USDC)
  2. Direct USDC transfer to their wallet address on Base
- There is no "Stripe payment method" vs "wallet payment method" - x402 is the only payment method

### x402 EIP-3009 Implementation Details

**CRITICAL: Both `Wallet` and `TestWallet` must use identical signing logic.**

The x402 payment uses EIP-3009 `TransferWithAuthorization` with EIP-712 typed data signing:

- **EIP-712 Types**: Use `apitypes.TypedData` from go-ethereum (NOT our custom `TypedData` struct with JSON hashing)
- **Domain**: `name="USD Coin"`, `version="2"`, `chainId`, `verifyingContract=tokenAddress`
- **Primary Type**: `TransferWithAuthorization` (NOT "Payment")
- **Value types**: Use `*math.HexOrDecimal256` for uint256 fields
- **Nonce format**: Use `hexutil.Encode(nonceBytes)` to get "0x..." hex string
- **V value**: go-ethereum returns 0/1, must adjust to 27/28 (`if sig[64] < 27 { sig[64] += 27 }`)

Reference implementations:
- [ethers.js example](https://github.com/brtvcl/eip-3009-transferWithAuthorization-example)
- [viem signature format](https://github.com/wevm/viem/blob/main/src/accounts/utils/sign.ts)
- [x402 TypeScript SDK](https://github.com/coinbase/x402/tree/main/typescript/packages/mechanisms/evm)

## Testing Requirements

**CRITICAL: All tests must actually run and pass before considering work complete.**

- Tests require Docker to be running (testcontainers spins up PostgreSQL)
- If Docker is not available, tests will be SKIPPED - this is NOT the same as passing
- **Never proceed with a commit if tests are skipped due to missing infrastructure**
- If you see "Docker is not available, skipping test" - STOP and start Docker first
- Pre-existing test failures must be noted and either fixed or explicitly acknowledged by the user

```bash
# FIRST: Ensure Docker is running (MANDATORY before running tests)
sudo systemctl start docker

# Verify Docker is running
docker info

# Run all tests (requires Docker)
go test ./...

# If Docker service doesn't exist, install it first:
# Arch: sudo pacman -S docker
# Ubuntu/Debian: sudo apt install docker.io
# Then add user to docker group: sudo usermod -aG docker $USER && newgrp docker
```

**DO NOT use `-short` flag** - this skips important integration tests. Always run the full test suite.

### Mocked vs Real Testing - CRITICAL DISTINCTION

**NEVER claim something "works" if it was only tested with mocks.** Be explicit about what has been actually tested:

1. **Real e2e tests** - Tests that hit actual external APIs (x402 facilitator, blockchain RPCs, etc.)
2. **Mocked tests** - Tests that mock external dependencies

When reporting test results, ALWAYS specify:
- "Tests pass WITH MOCKS for external services" (not fully verified)
- "Tests pass WITH REAL API calls" (actually verified)

**x402 Payment Testing:**
- Mocked facilitator tests do NOT verify the actual signature format, API format, or protocol compliance
- The ONLY way to verify x402 payments work is to test against the REAL x402.org facilitator
- Use `cmd/x402test/e2e.go` to test real x402 payments against production
- If tests mock the facilitator, they CANNOT catch EIP-712 signature format bugs, API format mismatches, etc.

## Database

PostgreSQL 16 with auto-migrations in `internal/db/migrations/`. Tables: accounts, sessions, usage, deposits.

## Deployment

- **API (Fly.io)**: `fly deploy` (configured in fly.toml)
- **Frontend (Cloudflare Pages)**: Builds automatically on push to master - hosted at `stronghold-bhj.pages.dev`
- **Docker Compose**: `docker-compose up -d` (local development only)
- Caddy provides auto HTTPS with Let's Encrypt

**IMPORTANT: Do NOT build frontend locally** (`bun run build`) unless debugging a remote build failure. Cloudflare Pages builds on push, so just commit and push frontend changes. Local builds are slow and unnecessary.

## Releases

To create a new CLI release:

```bash
git tag v1.0.0
git push origin v1.0.0
```

This triggers `.github/workflows/release.yml` which builds binaries for all platforms (linux/darwin × amd64/arm64) and publishes them to GitHub Releases.

Users can install with:
```bash
curl -fsSL https://getstronghold.xyz/install.sh | sh
```
