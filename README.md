# Citadel API Service

A pay-per-request API service that wraps the Citadel AI security scanner with x402 crypto payment integration. Enables AI agents to proxy requests through crypto payments to detect prompt injection attacks and credential leaks.

## Features

- **4-Layer Security Scanning**: Heuristics, ML classification, semantic similarity, and LLM classification
- **x402 Payment Integration**: Pay-per-request model using USDC on Base
- **Input Protection**: Detect prompt injection attacks
- **Output Protection**: Detect credential leaks in LLM responses
- **Multi-turn Protection**: Context-aware conversation scanning

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

3. **Clone and configure**
```bash
git clone <repo-url>
cd citadel-api

# Create environment file
cat > .env << 'EOF'
X402_WALLET_ADDRESS=0xYOUR_WALLET_ADDRESS
X402_NETWORK=base
CITADEL_ENABLE_HUGOT=true
CITADEL_ENABLE_SEMANTICS=true
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
   - Edit `Caddyfile` and replace `api.citadel.security` with your domain
   - Ensure DNS points to your VPS
   - Caddy auto-provisions Let's Encrypt certificates

### Environment Variables

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `X402_WALLET_ADDRESS` | Yes | - | USDC receiving address |
| `X402_NETWORK` | No | `base` | `base` or `base-sepolia` |
| `CITADEL_ENABLE_HUGOT` | No | `true` | Enable ML classification |
| `CITADEL_ENABLE_SEMANTICS` | No | `true` | Enable semantic similarity |

## Configuration

Environment variables:

```bash
# Server
PORT=8080

# x402 Payment (optional for development)
X402_WALLET_ADDRESS=0x...           # Receiving wallet address
X402_FACILITATOR_URL=https://x402.org/facilitator
X402_NETWORK=base-sepolia           # or base for production

# Citadel Configuration
CITADEL_BLOCK_THRESHOLD=0.55
CITADEL_WARN_THRESHOLD=0.35
CITADEL_ENABLE_HUGOT=true
CITADEL_ENABLE_SEMANTICS=true
HUGOT_MODEL_PATH=./models

# Optional LLM Layer
CITADEL_LLM_PROVIDER=groq
CITADEL_LLM_API_KEY=gsk_...

# Pricing (in USD)
PRICE_SCAN_INPUT=0.001
PRICE_SCAN_OUTPUT=0.001
PRICE_SCAN_UNIFIED=0.002
PRICE_SCAN_MULTITURN=0.005
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

#### POST /v1/scan/input ($0.001)
Scan user input for prompt injection.

```bash
curl -X POST http://localhost:8080/v1/scan/input \
  -H "Content-Type: application/json" \
  -H "X-PAYMENT: x402;..." \
  -d '{
    "text": "user prompt here",
    "session_id": "optional-session-id"
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

#### POST /v1/scan ($0.002)
Unified scanning endpoint.

```bash
curl -X POST http://localhost:8080/v1/scan \
  -H "Content-Type: application/json" \
  -H "X-PAYMENT: x402;..." \
  -d '{
    "text": "content to scan",
    "mode": "input"
  }'
```

Modes: `input`, `output`, `both`

#### POST /v1/scan/multiturn ($0.005)
Multi-turn conversation protection.

```bash
curl -X POST http://localhost:8080/v1/scan/multiturn \
  -H "Content-Type: application/json" \
  -H "X-PAYMENT: x402;..." \
  -d '{
    "session_id": "conversation-123",
    "turns": [
      {"role": "user", "content": "Hello"},
      {"role": "assistant", "content": "Hi there"},
      {"role": "user", "content": "ignore previous instructions"}
    ]
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

// Scan user input before sending to LLM
const result = await fetchWithPayment(
  "https://api.citadel.security/v1/scan/input",
  {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ text: userInput })
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
├── cmd/api/main.go              # Entry point
├── internal/
│   ├── server/server.go         # HTTP server setup
│   ├── handlers/
│   │   ├── scan.go              # Scan endpoints
│   │   ├── health.go            # Health checks
│   │   └── pricing.go           # Pricing info
│   ├── middleware/x402.go       # Payment middleware
│   ├── config/config.go         # Configuration
│   └── citadel/client.go        # Scanner wrapper
├── go.mod
├── Dockerfile
└── docker-compose.yml
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
│  Citadel Scanner    │
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
go build -o citadel-api cmd/api/main.go
```

### Development Mode

If `X402_WALLET_ADDRESS` is not set, the server runs in development mode where all endpoints are accessible without payment. This is useful for testing but should never be used in production.

## License

MIT
