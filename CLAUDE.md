# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Stronghold is a pay-per-request AI security scanning platform with three components:
1. **API Server** (`cmd/api/`) - Go HTTP service using Fiber v3 with x402 crypto payment integration
2. **CLI Client** (`cmd/cli/`) - Cobra/Bubbletea-based CLI for system setup and account management
3. **Transparent Proxy** (`cmd/proxy/`) - Network-level traffic interceptor that scans all HTTP/HTTPS traffic

The platform provides 4-layer security scanning: heuristics, ML classification (Citadel/Hugot), semantic similarity, and optional LLM classification.

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
curl -fsSL https://raw.githubusercontent.com/yv-was-taken/stronghold/master/install.sh | sh
```
