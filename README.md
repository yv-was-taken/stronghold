# Stronghold API Service

A pay-per-request API service that wraps the Stronghold AI security scanner with x402 crypto payment integration. Enables AI agents to proxy requests through crypto payments to detect prompt injection attacks and credential leaks.

## Client Proxy (NEW)

The Stronghold CLI provides a **transparent proxy** that intercepts ALL HTTP/HTTPS traffic at the network level, scanning content before it reaches your AI agents. This is designed for isolated machines running AI agents.

### Quick Start

```bash
# Check if your system is ready
stronghold doctor

# Install Stronghold (interactive setup)
sudo stronghold install

# Enable protection (starts transparent proxy)
sudo stronghold enable

# Check status
stronghold status

# Disable protection
sudo stronghold disable
```

### Installation

**One-line installer:**
```bash
curl -fsSL https://install.stronghold.security | sh
```

**Or build from source:**
```bash
git clone https://github.com/yv-was-taken/stronghold.git
cd stronghold
go build -o stronghold ./cmd/cli
go build -o stronghold-proxy ./cmd/proxy
```

### Prerequisites

- **OS**: Linux or macOS
- **Privileges**: Root/sudo required for `install`, `enable`, `disable`
- **Firewall**: iptables or nftables (Linux), pf (macOS) - usually pre-installed
- **Keyring** (Linux only): One of the following must be installed:
  - GNOME Keyring / Secret Service (`gnome-keyring`)
  - KWallet (pre-installed on KDE)
  - pass (`pass` password-store)

Run `stronghold doctor` to verify your system meets all requirements.

### CLI Commands

| Command | Description | Requires Root |
|---------|-------------|---------------|
| `stronghold doctor` | Check system prerequisites | No |
| `stronghold install` | Interactive installation | Yes |
| `stronghold enable` | Start proxy and enable traffic interception | Yes |
| `stronghold disable` | Stop proxy and restore direct access | Yes |
| `stronghold status` | Show proxy status and statistics | No |
| `stronghold logs` | View proxy logs | No |
| `stronghold account balance` | Check your account balance | No |
| `stronghold account deposit` | Show deposit options | No |
| `stronghold uninstall` | Remove Stronghold from system | Yes |

### How It Works

The transparent proxy uses **iptables/nftables** (Linux) or **pf** (macOS) to intercept traffic at the network level:

```
┌─────────────────────────────────────────────┐
│  Agent makes HTTP request                   │
│       │                                     │
│       ▼                                     │
│  Kernel intercepts (iptables/pf)            │
│       │                                     │
│       ▼                                     │
│  Stronghold Proxy (localhost:8080)          │
│       │                                     │
│       ├── Fetches content from destination  │
│       ├── Scans with Stronghold API         │
│       └── Returns ALLOW/WARN/BLOCK          │
│       │                                     │
│       ▼                                     │
│  Response returned to agent                 │
└─────────────────────────────────────────────┘
```

**Key features:**
- Cannot be bypassed by applications (unlike HTTP_PROXY env vars)
- Works for all processes automatically
- Adds `X-Stronghold-Decision` headers to responses
- Blocks malicious content before agents see it

### Account Management

Stronghold creates an account during installation to pay for API scanning. Your payment credentials are stored securely in your operating system's keyring.

**Check your balance:**
```bash
stronghold account balance
```

**Add funds:**
```bash
stronghold account deposit
```

This shows options to deposit via:
- **Dashboard**: Stripe, Coinbase Pay, or Moonpay (recommended)
- **Direct**: Send USDC to your account ID

**Security Notes:**
- Private keys never leave your device
- Credentials stored in OS-native keyring (macOS Keychain, Linux Secret Service/KWallet/pass, Windows Credential)
- Only your account ID is shared with the backend for linking
- Low balance warnings appear when below 1 USDC

### Architecture Overview

The project now consists of two main components:

1. **API Server** (`cmd/api/`) - The pay-per-request scanning service
2. **Client Proxy** (`cmd/cli/`, `cmd/proxy/`) - Local transparent proxy for agents

## Features

- **4-Layer Security Scanning**: Heuristics, ML classification, semantic similarity, and LLM classification
- **x402 Payment Integration**: Pay-per-request model using USDC on Base
- **Input Protection**: Detect prompt injection attacks
- **Output Protection**: Detect credential leaks in LLM responses

## Deployment

### VPS Deployment (Recommended)

1. **Provision a VPS** (Ubuntu 22.04+ recommended)
   - Minimum: 2 vCPU, 2GB RAM
   - Recommended: 4 vCPU, 4GB RAM for ML models

2. **Install Docker**
```bash
curl -fsSL https://get.docker.com | sh
sudo usermod -aG docker $USER
newgrp docker
```

### Fly.io Deployment (Recommended)

1. **Install Fly CLI**
```bash
curl -L https://fly.io/install.sh | sh
fly auth login
```

2. **Launch the app**
```bash
git clone https://github.com/yv-was-taken/stronghold.git
cd stronghold

fly launch --name stronghold-api --region iad
```

3. **Set secrets**
```bash
fly secrets set X402_WALLET_ADDRESS=0xYOUR_WALLET_ADDRESS
fly secrets set X402_NETWORK=base
fly secrets set STRONGHOLD_LLM_API_KEY=optional_api_key
```

4. **Deploy**
```bash
fly deploy
```

Your app will be available at `https://stronghold-api.fly.dev` (or your custom domain).

### VPS Deployment

1. **Provision a VPS** (Ubuntu 22.04+ recommended)
   - Minimum: 2 vCPU, 2GB RAM
   - Recommended: 4 vCPU, 4GB RAM for ML models

2. **Install Docker**
```bash
curl -fsSL https://get.docker.com | sh
sudo usermod -aG docker $USER
newgrp docker
```

3. **Clone and configure**
```bash
git clone https://github.com/yv-was-taken/stronghold.git
cd stronghold

# Create environment file
cat > .env << 'EOF'
X402_WALLET_ADDRESS=0xYOUR_WALLET_ADDRESS
X402_NETWORK=base
STRONGHOLD_ENABLE_HUGOT=true
STRONGHOLD_ENABLE_SEMANTICS=true
EOF
```

4. **Deploy**
```bash
# Build and start
docker-compose up -d

# With reverse proxy (Caddy)
docker-compose --profile with-proxy up -d
```

5. **Configure Caddy** (for HTTPS)
   - Edit `Caddyfile` and replace `api.stronghold.security` with your domain
   - Ensure DNS points to your VPS
   - Caddy auto-provisions Let's Encrypt certificates

### Environment Variables

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `X402_WALLET_ADDRESS` | Yes | - | USDC receiving address |
| `X402_NETWORK` | No | `base` | `base` or `base-sepolia` |
| `STRONGHOLD_ENABLE_HUGOT` | No | `true` | Enable ML classification |
| `STRONGHOLD_ENABLE_SEMANTICS` | No | `true` | Enable semantic similarity |

## Configuration

Environment variables:

```bash
# Server
PORT=8080

# x402 Payment (optional for development)
X402_WALLET_ADDRESS=0x...           # Receiving wallet address
X402_FACILITATOR_URL=https://x402.org/facilitator
X402_NETWORK=base-sepolia           # or base for production

# Stronghold Configuration
STRONGHOLD_BLOCK_THRESHOLD=0.55
STRONGHOLD_WARN_THRESHOLD=0.35
STRONGHOLD_ENABLE_HUGOT=true
STRONGHOLD_ENABLE_SEMANTICS=true
HUGOT_MODEL_PATH=./models

# Optional LLM Layer
STRONGHOLD_LLM_PROVIDER=groq
STRONGHOLD_LLM_API_KEY=gsk_...

# Pricing (in USD)
PRICE_SCAN_CONTENT=0.001
PRICE_SCAN_OUTPUT=0.001
```

## API Endpoints

### Public Endpoints (No Payment)

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/health` | GET | Health check |
| `/health/live` | GET | Liveness probe |
| `/health/ready` | GET | Readiness probe |
| `/v1/pricing` | GET | List endpoint pricing |

### Protected Endpoints (Payment Required)

#### POST /v1/scan/content ($0.001)
Scan external content for prompt injection.

```bash
curl -X POST http://localhost:8080/v1/scan/content \
  -H "Content-Type: application/json" \
  -H "X-PAYMENT: x402;..." \
  -d '{
    "text": "external content here",
    "source_url": "https://example.com",
    "source_type": "web_page"
  }'
```

Response:
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

#### POST /v1/scan/output ($0.001)
Scan LLM output for credential leaks.

```bash
curl -X POST http://localhost:8080/v1/scan/output \
  -H "Content-Type: application/json" \
  -H "X-PAYMENT: x402;..." \
  -d '{
    "text": "LLM response here"
  }'
```

## x402 Payment Flow

1. **Initial Request**: Client makes request without payment
2. **402 Response**: Server returns payment requirements
3. **Sign Payment**: Client signs EIP-712 payment authorization
4. **Retry**: Client retries with `X-PAYMENT` header
5. **Verify**: Server verifies payment via facilitator
6. **Response**: Server returns scan result with `X-PAYMENT-RESPONSE`

### Client Integration Example

```javascript
import { x402Client } from "x402-fetch";

const fetchWithPayment = x402Client({
  wallet: userWallet,
  network: "base"
});

// Scan external content before passing to LLM
const result = await fetchWithPayment(
  "https://api.stronghold.security/v1/scan/content",
  {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ text: externalContent })
  }
);

const scanResult = await result.json();
if (scanResult.decision === "BLOCK") {
  // Reject the input
  console.log("Threat detected:", scanResult.reason);
}
```

## Response Format

All scan endpoints return a standardized response:

```json
{
  "decision": "BLOCK|WARN|ALLOW",
  "scores": {
    "heuristic": 0.0-1.0,
    "ml_confidence": 0.0-1.0,
    "semantic": 0.0-1.0
  },
  "reason": "Human-readable explanation",
  "latency_ms": 15,
  "request_id": "uuid-for-tracing",
  "metadata": {} // Optional additional data
}
```

**Decisions:**
- `ALLOW`: No threats detected, proceed
- `WARN`: Elevated risk, review recommended
- `BLOCK`: High risk, reject the request

## Project Structure

```
.
├── cmd/
│   ├── api/main.go              # API server entry point
│   ├── cli/main.go              # CLI client entry point
│   └── proxy/main.go            # Proxy daemon entry point
├── internal/
│   ├── server/server.go         # HTTP server setup
│   ├── handlers/                # API endpoints
│   │   ├── scan.go
│   │   ├── health.go
│   │   └── pricing.go
│   ├── middleware/x402.go       # Payment middleware
│   ├── config/config.go         # Server configuration
│   ├── stronghold/client.go     # Scanner wrapper
│   ├── wallet/                  # Wallet management
│   │   ├── wallet.go            # OS keyring wallet operations
│   │   └── x402.go              # x402 payment creation/verification
│   ├── cli/                     # CLI implementation
│   │   ├── config.go            # CLI configuration
│   │   ├── doctor.go            # Prerequisites check
│   │   ├── install.go           # Interactive installer
│   │   ├── enable.go            # Enable proxy
│   │   ├── disable.go           # Disable proxy
│   │   ├── status.go            # Status display
│   │   ├── uninstall.go         # Uninstall
│   │   ├── wallet.go            # Wallet CLI commands
│   │   ├── service.go           # System service management
│   │   └── transparent.go       # Transparent proxy (iptables/pf)
│   └── proxy/                   # Proxy implementation
│       ├── server.go            # HTTP/HTTPS proxy server
│       └── scanner.go           # API client for scanning
├── go.mod
├── Dockerfile
├── docker-compose.yml
└── install.sh                   # One-line installer
```

## Architecture

```
AI Agent Clients
       │
       │ 1. Request with X-PAYMENT header
       ▼
┌─────────────────────┐
│  x402 Middleware    │
│  - Verify payment   │
└──────────┬──────────┘
           │ 2. Payment verified
           ▼
┌─────────────────────┐
│  Stronghold Scanner │
│  - 4-layer scanning │
└──────────┬──────────┘
           │ 3. Scan result
           ▼
     (Client receives)
```

## Development

### Running Tests

```bash
go test ./...
```

### Building

```bash
go build -o stronghold-api cmd/api/main.go
```

### Development Mode

If `X402_WALLET_ADDRESS` is not set, the server runs in development mode where all endpoints are accessible without payment. This is useful for testing but should never be used in production.

## License

MIT
