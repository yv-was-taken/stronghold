# Stronghold

Protect AI agents from prompt injection attacks and credential leaks.

## The Problem

AI agents that read external content are vulnerable to prompt injection attacks. When an agent fetches a webpage, email, or API response, that content may contain malicious instructions designed to hijack the agent's behavior.

```
1. Attacker embeds instructions: "Ignore previous instructions..."
2. Agent fetches content (webpage, email, document)
3. Agent reads malicious content into its context
4. Agent follows attacker's instructions instead of user's
```

## Two-Way Protection

Stronghold protects both directions of data flow:

```
External content → [PROXY scans] → Agent → [API scans] → Output to user
                    ↑                        ↑
                 Injection               Credential leaks
                 (INCOMING)              (OUTGOING)
```

**Proxy** scans INCOMING content — blocks prompt injection before the agent sees it
**API** scans OUTGOING content — catches credential leaks in agent responses

Both require an account with USDC balance. $0.001 per scan.

---

## 1. Transparent Proxy (Incoming Content)

The proxy intercepts ALL HTTP/HTTPS traffic at the network level, scanning content **before** it reaches your AI agents.

### Why Network-Level Protection?

Traditional security that requires the agent to call an API has a fundamental flaw:

- To call a security API, the agent must first **READ** the content
- At that moment, prompt injection can already affect the agent
- The agent might "forget" to call the API or ignore the result
- The attack has already succeeded before any scan occurs

The transparent proxy solves this by operating **outside** the agent's cognition:

- Content is scanned **before** the agent receives it
- Malicious content is blocked at the network level
- The agent never processes threats it cannot see
- **Cannot be bypassed by prompt injection**

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
- **Firewall**: iptables or nftables (Linux), pf (macOS) — usually pre-installed
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

---

## 2. Output Scanning API (Outgoing Content)

The API scans agent responses **before** they're sent to users, catching accidentally exposed credentials.

### POST /v1/scan/output — Credential Leak Detection

Use this to check agent output for accidentally exposed:
- API keys and tokens
- Passwords and secrets
- Database connection strings
- AWS credentials, private keys

```bash
curl -X POST https://api.stronghold.security/v1/scan/output \
  -H "Content-Type: application/json" \
  -H "X-PAYMENT: x402;..." \
  -d '{
    "text": "Here is the config: DB_PASSWORD=secret123"
  }'
```

### POST /v1/scan/content — Prompt Injection Detection

> **⚠️ We strongly recommend using the transparent proxy instead.**
>
> This endpoint scans text for prompt injection, but by the time you call it, your agent has already read the content. The proxy blocks threats **before** your agent sees them.
>
> Use this endpoint only if you cannot install the proxy (serverless functions, sandboxed containers, etc.).

```bash
curl -X POST https://api.stronghold.security/v1/scan/content \
  -H "Content-Type: application/json" \
  -H "X-PAYMENT: x402;..." \
  -d '{
    "text": "external content here",
    "source_url": "https://example.com",
    "source_type": "web_page"
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

**Decisions:**
- `ALLOW`: No threats detected, proceed
- `WARN`: Elevated risk, review recommended
- `BLOCK`: High risk, reject the request

---

## 3. Account & Funding

Both the proxy and API require a funded account.

### Create an Account

**Option 1: Dashboard**
Visit https://stronghold.security/dashboard

**Option 2: CLI**
Run `sudo stronghold install` — the installer creates a local wallet and registers it.

### Check Balance & Add Funds

```bash
# Check your balance
stronghold account balance

# Add funds
stronghold account deposit
```

Deposit options:
- **Dashboard**: Stripe, Coinbase Pay, or Moonpay (recommended)
- **Direct**: Send USDC on Base to your account address

### Pricing

| Endpoint | Price |
|----------|-------|
| `/v1/scan/content` | $0.001 |
| `/v1/scan/output` | $0.001 |

### Security Notes

- Private keys never leave your device
- Credentials stored in OS-native keyring (macOS Keychain, Linux Secret Service/KWallet/pass)
- Only your account ID is shared with the backend for linking
- Low balance warnings appear when below 1 USDC

---

## API Reference

### Public Endpoints (No Payment)

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/health` | GET | Health check |
| `/health/live` | GET | Liveness probe |
| `/health/ready` | GET | Readiness probe |
| `/v1/pricing` | GET | List endpoint pricing |

### Protected Endpoints (Payment Required)

| Endpoint | Method | Price | Description |
|----------|--------|-------|-------------|
| `/v1/scan/content` | POST | $0.001 | Prompt injection detection |
| `/v1/scan/output` | POST | $0.001 | Credential leak detection |

---

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

// Scan output for credential leaks before sending to user
const result = await fetchWithPayment(
  "https://api.stronghold.security/v1/scan/output",
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

## Deployment

### Fly.io (Recommended)

```bash
# Install Fly CLI
curl -L https://fly.io/install.sh | sh
fly auth login

# Launch the app
git clone https://github.com/yv-was-taken/stronghold.git
cd stronghold
fly launch --name stronghold-api --region iad

# Set secrets
fly secrets set X402_WALLET_ADDRESS=0xYOUR_WALLET_ADDRESS
fly secrets set X402_NETWORK=base

# Deploy
fly deploy
```

### Docker Compose

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

# Build and start
docker-compose up -d

# With reverse proxy (Caddy for HTTPS)
docker-compose --profile with-proxy up -d
```

### Environment Variables

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `X402_WALLET_ADDRESS` | Yes* | - | USDC receiving address |
| `X402_NETWORK` | No | `base` | `base` or `base-sepolia` |
| `STRONGHOLD_ENABLE_HUGOT` | No | `true` | Enable ML classification |
| `STRONGHOLD_ENABLE_SEMANTICS` | No | `true` | Enable semantic similarity |
| `STRONGHOLD_BLOCK_THRESHOLD` | No | `0.55` | Score threshold for BLOCK |
| `STRONGHOLD_WARN_THRESHOLD` | No | `0.35` | Score threshold for WARN |

*If `X402_WALLET_ADDRESS` is not set, the server runs in development mode without payments.

---

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
│   ├── middleware/x402.go       # Payment middleware
│   ├── stronghold/client.go     # Scanner wrapper
│   ├── wallet/                  # Wallet management
│   ├── cli/                     # CLI implementation
│   └── proxy/                   # Proxy implementation
├── web/                         # Frontend (Next.js)
├── Dockerfile
├── docker-compose.yml
└── install.sh                   # One-line installer
```

## License

MIT
