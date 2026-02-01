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
go test ./internal/handlers -run TestScanInput

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
- `/v1/scan/input` - Prompt injection detection ($0.001)
- `/v1/scan/output` - Credential leak detection ($0.001)
- `/v1/scan` - Unified scanning ($0.002)
- `/v1/scan/multiturn` - Multi-turn conversation protection ($0.005)

## Database

PostgreSQL 16 with auto-migrations in `internal/db/migrations/`. Tables: accounts, sessions, usage, deposits.

## Deployment

- **Fly.io**: `fly deploy` (configured in fly.toml)
- **Docker Compose**: `docker-compose up -d`
- Caddy provides auto HTTPS with Let's Encrypt
