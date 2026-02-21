<p align="center">
  <img src="assets/logo.png" width="120" alt="Stronghold">
</p>

<h1 align="center">Stronghold</h1>

<p align="center">
  <strong>Enterprise Security for AI Infrastructure</strong>
</p>

<p align="center">
  Protect AI agents from prompt injection attacks and credential leaks.<br>
  Stronghold intercepts and scans all traffic through a transparent proxy, blocking threats before they reach your models.
</p>

<p align="center">
  A pay-per-request security API built on the <a href="https://github.com/citadel-ai/citadel">Citadel AI security scanner</a> with <a href="https://www.x402.org/">x402</a> payment protocol integration.
</p>

<p align="center">
  <a href="https://github.com/yv-was-taken/stronghold/actions"><img src="https://img.shields.io/github/actions/workflow/status/yv-was-taken/stronghold/test.yml?branch=master&label=tests" alt="Tests"></a>
  <a href="https://goreportcard.com/report/github.com/yv-was-taken/stronghold"><img src="https://goreportcard.com/badge/github.com/yv-was-taken/stronghold" alt="Go Report Card"></a>
  <a href="https://go.dev/"><img src="https://img.shields.io/badge/go-1.24+-00ADD8?logo=go&logoColor=white" alt="Go 1.24+"></a>
  <a href="https://github.com/yv-was-taken/stronghold/releases"><img src="https://img.shields.io/github/v/release/yv-was-taken/stronghold" alt="Release"></a>
  <a href="https://github.com/yv-was-taken/stronghold/blob/master/LICENSE"><img src="https://img.shields.io/github/license/yv-was-taken/stronghold" alt="License"></a>
</p>

<br>

## Quick Start

```bash
curl -fsSL https://getstronghold.xyz/install.sh | sh
stronghold doctor
sudo stronghold init
sudo stronghold enable
```

All HTTP/HTTPS traffic is now routed through the transparent proxy for real-time security scanning.

---

## Features

- **Multi-Layer Detection**: Four-layer scanning pipeline (heuristics, ML classification, semantic similarity, and optional LLM classification) delivers sub-50ms latency
- **Network-Level Protection**: Transparent proxy intercepts traffic at the kernel level, blocking threats before they reach your models
- **Bidirectional Scanning**: Detects prompt injection on inbound content and credential leaks on outbound responses
- **Pay-Per-Request Pricing**: $0.001 per scan via x402 protocol using USDC on Base, with no API keys or subscriptions required
- **Open Source**: MIT licensed and fully self-hostable

---

## Table of Contents

- [How It Works](#how-it-works)
- [Installation](#installation)
- [CLI Reference](#cli-reference)
  - [Configuring Scan Behavior](#configuring-scan-behavior)
  - [Response Headers for Agentic Integration](#response-headers-for-agentic-integration)
- [API Endpoints](#api-endpoints)
- [Account & Funding](#account--funding)
- [x402 Payment Flow](#x402-payment-flow)
- [Deployment](#deployment)
- [Project Structure](#project-structure)
- [License](#license)

---

## How It Works

Stronghold operates at the network level, scanning all content before it reaches the AI agent.

### Why Network-Level Protection?

Traditional security approaches require the agent to invoke a scanning API. However, to make that call, the agent must first read the content into its context window. At that point, prompt injection can already influence the agent's behavior, causing it to ignore scan results or fail to invoke the security API entirely.

The transparent proxy architecture eliminates this vulnerability:

- Content is scanned before the agent receives it
- Malicious content is blocked at the kernel level
- Agents never process threats they cannot observe
- Protection cannot be bypassed by prompt injection

### Architecture

```
┌─────────────┐      ┌─────────────┐      ┌─────────────┐
│   Agent     │      │   Kernel    │      │  Stronghold │
│  (any app)  │ ──▶  │  iptables   │ ──▶  │    Proxy    │
└─────────────┘      │     pf      │      │ :8402       │
                     └─────────────┘      └──────┬──────┘
                                                 │
                     ┌───────────────────────────┼───────────────────────────┐
                     │                           ▼                           │
                     │  ┌─────────────┐    ┌───────────┐    ┌─────────────┐  │
                     │  │   Fetch     │ ─▶ │   Scan    │ ─▶ │   Return    │  │
                     │  │   content   │    │   (API)   │    │   result    │  │
                     │  └─────────────┘    └───────────┘    └─────────────┘  │
                     │                                                       │
                     │              ALLOW / WARN / BLOCK                     │
                     └───────────────────────────────────────────────────────┘
```

### Bidirectional Protection

```
                    ┌──────────────────┐
 External Content   │                  │   Agent Output
 (web, email, API)  │    AI  Agent     │   (to user)
        │           │                  │        │
        ▼           └──────────────────┘        ▼
   ┌─────────┐                             ┌─────────┐
   │  PROXY  │ ──── safe content ────────▶ │   API   │
   │  SCAN   │                             │  SCAN   │
   └─────────┘                             └─────────┘
        │                                       │
        ▼                                       ▼
   Prompt injection                       Credential leak
   detection                              detection
```

| Layer | Direction | Threat Detection |
|-------|-----------|------------------|
| Proxy | Inbound | Prompt injection attacks in external content |
| API | Outbound | Credential leaks in agent responses |

---

## Installation

### One-Line Installer

```bash
curl -fsSL https://getstronghold.xyz/install.sh | sh
```

### Build from Source

```bash
git clone https://github.com/yv-was-taken/stronghold.git
cd stronghold
go build -o stronghold ./cmd/cli
go build -o stronghold-proxy ./cmd/proxy
```

### System Requirements

- **Operating System**: Linux or macOS
- **Privileges**: Root access required for `init`, `enable`, and `disable` commands
- **Firewall**: iptables or nftables (Linux), pf (macOS)
- **Keyring** (Linux only): gnome-keyring, KWallet, or pass

Run `stronghold doctor` to verify that all system requirements are met.

---

## CLI Reference

| Command | Description | Root Required |
|---------|-------------|---------------|
| `stronghold doctor` | Verify system requirements | No |
| `stronghold init` | Initialize installation and configure wallet | Yes |
| `stronghold enable` | Start proxy and enable traffic interception | Yes |
| `stronghold disable` | Stop proxy and restore direct network access | Yes |
| `stronghold status` | Display proxy status and statistics | No |
| `stronghold health` | Check API and Base/Solana RPC health | No |
| `stronghold logs` | View proxy logs | No |
| `stronghold account balance` | Display current account balance | No |
| `stronghold account deposit` | Display deposit options | No |
| `stronghold wallet list` | List configured Base/Solana wallet addresses | No |
| `stronghold wallet balance` | Display per-chain wallet balances | No |
| `stronghold wallet export` | Export private key for backup | No |
| `stronghold wallet replace <evm\|solana>` | Replace wallet by chain | No |
| `stronghold wallet link` | Register local wallet addresses with server | No |
| `stronghold config get [key]` | Display configuration value(s) | No |
| `stronghold config set <key> <value>` | Update a configuration value | No |
| `stronghold uninstall` | Remove Stronghold from the system | Yes |

### Wallet Commands

```bash
# List configured wallets
stronghold wallet list

# Show per-chain balances
stronghold wallet balance

# Replace specific wallet by chain (positional)
stronghold wallet replace evm
stronghold wallet replace solana

```

### Configuring Scan Behavior

Control how the proxy handles scan results without modifying the scanning itself:

```bash
# View all scanning configuration
stronghold config get scanning

# Never block content (headers still show scan results)
stronghold config set scanning.content.action_on_block allow
stronghold config set scanning.content.action_on_warn allow

# Strict mode - block even warnings
stronghold config set scanning.content.action_on_warn block

# Disable content scanning entirely
stronghold config set scanning.content.enabled false
```

Configuration file location: `~/.stronghold/config.yaml`

Example configuration:
```yaml
scanning:
  content:
    enabled: true
    action_on_warn: warn    # allow | warn | block
    action_on_block: block  # allow | warn | block
  output:
    enabled: true           # reserved for future output policy
    action_on_warn: warn    # currently not enforced by proxy runtime
    action_on_block: block  # currently not enforced by proxy runtime
```

### Response Headers for Agentic Integration

When the proxy scans content, headers are added to every response:

| Header | Description |
|--------|-------------|
| `X-Stronghold-Decision` | Scan result: `ALLOW`, `WARN`, or `BLOCK` |
| `X-Stronghold-Action` | Proxy action taken: `allow`, `warn`, or `block` |
| `X-Stronghold-Reason` | Human-readable explanation (if flagged) |
| `X-Stronghold-Score` | Combined threat score (0.00 - 1.00) |

Headers are present even when content is not blocked, enabling agents to observe scan results programmatically.

---

## API Endpoints

### Public Endpoints

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/health` | GET | Health check |
| `/health/live` | GET | Liveness probe for orchestration |
| `/health/ready` | GET | Readiness probe for orchestration |
| `/v1/pricing` | GET | Retrieve endpoint pricing |

### Protected Endpoints

Payment via x402 protocol is required for the following endpoints.

| Endpoint | Method | Price | Description |
|----------|--------|-------|-------------|
| `/v1/scan/content` | POST | $0.001 | Prompt injection detection |
| `/v1/scan/output` | POST | $0.001 | Credential leak detection |

**Money field format**:
- Canonical amount fields are string-encoded microUSDC (1 microUSDC = 0.000001 USDC)
- `/v1/pricing` includes both `price_micro_usdc` (canonical) and `price_usd` (human-readable)

**Breaking change (February 18, 2026)**:
- Money fields in API responses moved from JSON numbers to string-encoded microUSDC values
- Clients parsing response money fields as JSON numbers must migrate

### POST /v1/scan/content

Scans external content for prompt injection attacks. The transparent proxy invokes this endpoint automatically.

> **Note**: Direct API integration is recommended only when the proxy cannot be deployed (e.g., serverless environments, sandboxed containers). The proxy provides stronger protection by scanning content before it enters the agent's context.

```bash
curl -X POST https://api.getstronghold.xyz/v1/scan/content \
  -H "Content-Type: application/json" \
  -H "X-PAYMENT: x402;..." \
  -d '{
    "text": "external content to scan",
    "source_url": "https://example.com",
    "source_type": "web_page"
  }'
```

### POST /v1/scan/output

Scans agent responses for credential leaks before delivery to end users. Detects API keys, passwords, database connection strings, cloud provider credentials, and private keys.

```bash
curl -X POST https://api.getstronghold.xyz/v1/scan/output \
  -H "Content-Type: application/json" \
  -H "X-PAYMENT: x402;..." \
  -d '{
    "text": "Here is the config: DB_PASSWORD=secret123"
  }'
```

### Response Format

```json
{
  "decision": "BLOCK",
  "scores": {
    "heuristic": 0.85,
    "ml_confidence": 0.92,
    "semantic": 0.75
  },
  "reason": "High heuristic score - possible prompt injection",
  "latency_ms": 15,
  "request_id": "uuid-for-tracing"
}
```

**Decision Values**:
- `ALLOW`: No threats detected; content is safe to process
- `WARN`: Elevated risk detected; manual review recommended
- `BLOCK`: High-confidence threat detected; request should be rejected

---

## Account & Funding

Both the proxy and API require an account with a USDC balance.

### Account Creation

**Via Dashboard**: Visit https://getstronghold.xyz/dashboard

**Via CLI**: Run `sudo stronghold init` to create a local wallet and register it with the service.

### Balance Management

```bash
stronghold account balance    # Display current balance
stronghold account deposit    # Display deposit options
```

### Deposit Methods

- **Dashboard**: Stripe (card to USDC)
- **Direct Transfer**: Send USDC on Base to your account address

### Pricing

| Endpoint | Price per Request |
|----------|-------------------|
| `/v1/scan/content` | $0.001 |
| `/v1/scan/output` | $0.001 |

### Credential Security

- Private keys are stored locally and never transmitted to external services
- Credentials are secured in the OS-native keyring (macOS Keychain, Linux Secret Service/KWallet/pass)
- Only the public account identifier is shared with the backend for account linking

---

## x402 Payment Flow

1. **Initial Request**: Client sends request without payment header
2. **402 Response**: Server returns payment requirements in the response
3. **Payment Signing**: Client signs an EIP-712 payment authorization
4. **Authenticated Retry**: Client retries request with `X-PAYMENT` header
5. **Payment Verification**: Server verifies payment via the x402 facilitator
6. **Response Delivery**: Server returns scan result with `X-PAYMENT-RESPONSE` header

### Client Integration

```javascript
import { x402Client } from "x402-fetch";

const fetchWithPayment = x402Client({
  wallet: userWallet,
  network: "base"
});

// Scan agent output before delivery to the end user
const result = await fetchWithPayment(
  "https://api.getstronghold.xyz/v1/scan/output",
  {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ text: agentResponse })
  }
);

const scanResult = await result.json();
if (scanResult.decision === "BLOCK") {
  console.log("Credential leak detected:", scanResult.reason);
}
```

---

## Self-Hosting

Stronghold is fully self-hostable. Docker Compose brings up the complete stack: PostgreSQL, the API server, and the x402 payment facilitator.

```bash
git clone https://github.com/yv-was-taken/stronghold.git && cd stronghold
cp .env.example .env  # Configure environment variables
docker-compose up -d

# With HTTPS via Caddy reverse proxy
docker-compose --profile with-proxy up -d
```

### Configuration

Copy `.env.example` to `.env` and configure the required values. See `.env.example` for full documentation.

**API Server:**

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `X402_EVM_WALLET_ADDRESS` | Yes* | - | EVM USDC receiving address (Base) |
| `X402_SOLANA_WALLET_ADDRESS` | No | - | Solana USDC receiving address |
| `X402_NETWORKS` | No | `base` | Supported networks (comma-separated): `base`, `solana`, `base-sepolia`, `solana-devnet` |
| `STRONGHOLD_ENABLE_HUGOT` | No | `true` | Enable ML classification layer |
| `STRONGHOLD_ENABLE_SEMANTICS` | No | `true` | Enable semantic similarity layer |
| `STRONGHOLD_BLOCK_THRESHOLD` | No | `0.55` | Score threshold for BLOCK decisions |
| `STRONGHOLD_WARN_THRESHOLD` | No | `0.35` | Score threshold for WARN decisions |

*When no wallet addresses are configured, the server runs in development mode without payment verification.

### Migration rollout note (003_usdc_microusdc)

- This migration drops and renames columns in one transaction; it is not safe for zero-downtime rolling deploys with mixed old/new app versions.
- Apply migration and deploy updated API together (or accept a brief maintenance window).
- The transaction can hold table locks for the duration of migration on large datasets; schedule accordingly.

**x402 Facilitator** (settles payments on-chain):

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `FACILITATOR_EVM_PRIVATE_KEY` | Yes | - | Private key for the EVM settlement wallet (funded with ETH for gas) |
| `RPC_URL_BASE` | Yes | - | Base mainnet RPC endpoint |
| `RPC_URL_BASE_SEPOLIA` | No | - | Base Sepolia RPC endpoint (for testnet) |

The facilitator wallet needs a small ETH balance on Base for gas (~0.01 ETH lasts ~90,000 settlements at current gas prices). You can use any RPC provider; [Alchemy's free tier](https://dashboard.alchemy.com/) (30M compute units/month) is sufficient for most deployments.

---

## Project Structure

```
.
├── cmd/
│   ├── api/           # API server entry point
│   ├── cli/           # CLI client entry point
│   └── proxy/         # Proxy daemon entry point
├── facilitator/       # x402 payment settlement service
├── internal/
│   ├── server/        # HTTP server setup
│   ├── handlers/      # API endpoint handlers
│   ├── middleware/    # x402 payment verification
│   ├── stronghold/    # Citadel scanner integration
│   ├── wallet/        # Wallet management
│   ├── cli/           # CLI implementation
│   └── proxy/         # Proxy implementation
├── web/               # Frontend (Next.js)
└── install.sh         # One-line installer script
```

---

## Built With

- [Citadel](https://github.com/citadel-ai/citadel) - AI security scanner with multi-layer threat detection
- [x402](https://www.x402.org/) - HTTP 402 payment protocol on Base
- [Fiber](https://gofiber.io/) - High-performance HTTP framework for Go
- [Bubbletea](https://github.com/charmbracelet/bubbletea) - Terminal UI framework for the CLI

---

## License

MIT
