#!/bin/bash
# Stronghold Facilitator Infrastructure Verification
#
# Verifies the remote production infrastructure is fully configured:
#   - Fly.io secrets set on both stronghold-api and stronghold-facilitator
#   - Remote facilitator and API health endpoints responding
#   - RPC endpoints reachable and returning correct chain data
#   - Settlement wallets funded with gas tokens (ETH on Base, SOL on Solana)
#   - API reports facilitator connectivity is healthy
#   - USDC token contracts valid on-chain
#
# Required:
#   - flyctl CLI installed and authenticated
#   - curl and jq installed
#   - FACILITATOR_EVM_ADDRESS and FACILITATOR_SOLANA_ADDRESS env vars
#     (public addresses of the facilitator settlement wallets)
#
# Usage:
#   FACILITATOR_EVM_ADDRESS=0x... FACILITATOR_SOLANA_ADDRESS=... ./scripts/verify-facilitator.sh

set -euo pipefail

# ── Colors ───────────────────────────────────────────────────────────────────

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
DIM='\033[2m'
BOLD='\033[1m'
NC='\033[0m'

# ── Config ───────────────────────────────────────────────────────────────────

FACILITATOR_APP="stronghold-facilitator"
API_APP="stronghold-api"

API_URL="https://api.getstronghold.xyz"
FACILITATOR_URL="https://${FACILITATOR_APP}.fly.dev"

# Mainnet
BASE_RPC_DEFAULT="https://mainnet.base.org"
SOLANA_RPC_DEFAULT="https://api.mainnet-beta.solana.com"
BASE_CHAIN_ID=8453
BASE_USDC="0x833589fCD6eDb6E08f4c7C32D4f71b54bdA02913"
SOLANA_USDC_MINT="EPjFWdd5AufqSSqeM2qN1xzybapC8G4wEGGkZwyTDt1v"

# Minimum balances
MIN_ETH_WEI="1000000000000000"  # 0.001 ETH
MIN_SOL_LAMPORTS=10000000       # 0.01 SOL

# Wallet addresses (from env)
EVM_WALLET="${FACILITATOR_EVM_ADDRESS:-}"
SOL_WALLET="${FACILITATOR_SOLANA_ADDRESS:-}"

# ── Helpers ──────────────────────────────────────────────────────────────────

PASS_COUNT=0
FAIL_COUNT=0
WARN_COUNT=0
ACTIONS=()

pass() {
    echo -e "  ${GREEN}✓${NC} $1"
    PASS_COUNT=$((PASS_COUNT + 1))
}

fail() {
    echo -e "  ${RED}✗${NC} $1"
    FAIL_COUNT=$((FAIL_COUNT + 1))
    if [ -n "${2:-}" ]; then
        ACTIONS+=("$2")
    fi
}

warn() {
    echo -e "  ${YELLOW}!${NC} $1"
    WARN_COUNT=$((WARN_COUNT + 1))
    if [ -n "${2:-}" ]; then
        ACTIONS+=("$2")
    fi
}

info() {
    echo -e "  ${DIM}$1${NC}"
}

section() {
    echo ""
    echo -e "${BOLD}${BLUE}[$1]${NC}"
}

# ── Header ───────────────────────────────────────────────────────────────────

echo ""
echo -e "${BOLD}Stronghold Facilitator Infrastructure Verification${NC}"
echo -e "${DIM}$(date -u '+%Y-%m-%d %H:%M:%S UTC')${NC}"

# ══════════════════════════════════════════════════════════════════════════════
# 1. Prerequisites
# ══════════════════════════════════════════════════════════════════════════════

section "Prerequisites"

for tool in curl jq flyctl; do
    if ! command -v "$tool" &>/dev/null; then
        fail "$tool not installed" "Install $tool"
        echo -e "${RED}Cannot continue without $tool. Aborting.${NC}"
        exit 1
    fi
    pass "$tool installed"
done

if ! flyctl auth whoami &>/dev/null 2>&1; then
    fail "flyctl not authenticated" "Run: flyctl auth login"
    echo -e "${RED}Cannot continue without flyctl auth. Aborting.${NC}"
    exit 1
fi
pass "flyctl authenticated"

# ══════════════════════════════════════════════════════════════════════════════
# 2. Fly.io Deployment Status
# ══════════════════════════════════════════════════════════════════════════════

section "Fly.io Deployment Status"

check_fly_app() {
    local app="$1"
    if flyctl status -a "$app" &>/dev/null 2>&1; then
        local state
        state=$(flyctl status -a "$app" --json 2>/dev/null | jq -r '.Machines[0].state // "unknown"' 2>/dev/null || echo "unknown")
        if [ "$state" = "started" ] || [ "$state" = "running" ]; then
            pass "$app is running (state: $state)"
        else
            fail "$app machine state: $state" "Check app: flyctl status -a $app"
        fi
    else
        fail "$app not found on Fly.io" "Deploy: flyctl deploy -a $app"
    fi
}

check_fly_app "$FACILITATOR_APP"
check_fly_app "$API_APP"

# ══════════════════════════════════════════════════════════════════════════════
# 3. Fly.io Secrets Audit
# ══════════════════════════════════════════════════════════════════════════════

section "Fly.io Secrets — $FACILITATOR_APP"

FACILITATOR_SECRETS=$(flyctl secrets list -a "$FACILITATOR_APP" --json 2>/dev/null || echo "[]")

check_secret() {
    local app="$1" name="$2" desc="$3" fix="$4" json="$5"
    # flyctl secrets list --json uses lowercase "name" field
    if echo "$json" | jq -e ".[] | select(.name == \"$name\")" &>/dev/null; then
        pass "$desc"
    else
        fail "$desc — NOT SET" "$fix"
    fi
}

check_secret "$FACILITATOR_APP" "FACILITATOR_EVM_PRIVATE_KEY" "FACILITATOR_EVM_PRIVATE_KEY" \
    "flyctl secrets set FACILITATOR_EVM_PRIVATE_KEY=0x... -a $FACILITATOR_APP" "$FACILITATOR_SECRETS"

check_secret "$FACILITATOR_APP" "FACILITATOR_SOLANA_PRIVATE_KEY" "FACILITATOR_SOLANA_PRIVATE_KEY" \
    "flyctl secrets set FACILITATOR_SOLANA_PRIVATE_KEY=... -a $FACILITATOR_APP" "$FACILITATOR_SECRETS"

check_secret "$FACILITATOR_APP" "RPC_URL_BASE" "RPC_URL_BASE" \
    "flyctl secrets set RPC_URL_BASE=https://base-mainnet.g.alchemy.com/v2/KEY -a $FACILITATOR_APP" "$FACILITATOR_SECRETS"

check_secret "$FACILITATOR_APP" "RPC_URL_SOLANA" "RPC_URL_SOLANA" \
    "flyctl secrets set RPC_URL_SOLANA=https://mainnet.helius-rpc.com/?api-key=KEY -a $FACILITATOR_APP" "$FACILITATOR_SECRETS"

section "Fly.io Secrets — $API_APP"

API_SECRETS=$(flyctl secrets list -a "$API_APP" --json 2>/dev/null || echo "[]")

check_secret "$API_APP" "X402_FACILITATOR_URL" "X402_FACILITATOR_URL" \
    "flyctl secrets set X402_FACILITATOR_URL=http://$FACILITATOR_APP.internal:8402 -a $API_APP" "$API_SECRETS"

# Accept either X402_EVM_WALLET_ADDRESS (new) or X402_WALLET_ADDRESS (legacy)
if echo "$API_SECRETS" | jq -e '.[] | select(.name == "X402_EVM_WALLET_ADDRESS" or .name == "X402_WALLET_ADDRESS")' &>/dev/null; then
    pass "X402 EVM wallet address"
else
    fail "X402 EVM wallet address — NOT SET" \
        "flyctl secrets set X402_EVM_WALLET_ADDRESS=0x... -a $API_APP"
fi

check_secret "$API_APP" "X402_SOLANA_WALLET_ADDRESS" "X402_SOLANA_WALLET_ADDRESS" \
    "flyctl secrets set X402_SOLANA_WALLET_ADDRESS=... -a $API_APP" "$API_SECRETS"

check_secret "$API_APP" "X402_NETWORKS" "X402_NETWORKS" \
    "flyctl secrets set X402_NETWORKS=base,solana -a $API_APP" "$API_SECRETS"

check_secret "$API_APP" "X402_SOLANA_FEE_PAYER" "X402_SOLANA_FEE_PAYER" \
    "flyctl secrets set X402_SOLANA_FEE_PAYER=<facilitator-solana-pubkey> -a $API_APP" "$API_SECRETS"

# ══════════════════════════════════════════════════════════════════════════════
# 4. Remote Health Checks
# ══════════════════════════════════════════════════════════════════════════════

section "Facilitator Health ($FACILITATOR_URL)"

HEALTH_RESPONSE=$(curl -sf --max-time 10 "$FACILITATOR_URL/health" 2>/dev/null || echo "")

if [ -n "$HEALTH_RESPONSE" ]; then
    pass "Facilitator /health responding"

    # Extract signer addresses from health response for balance checks
    HEALTH_EVM_SIGNER=$(echo "$HEALTH_RESPONSE" | jq -r '.signers["eip155:8453"][0] // ""' 2>/dev/null)
    HEALTH_SOL_SIGNER=$(echo "$HEALTH_RESPONSE" | jq -r '.signers["solana:5eykt4UsFv8P8NJdTREpY1vzqKqZKvdp"][0] // ""' 2>/dev/null)

    if [ -n "$HEALTH_EVM_SIGNER" ]; then
        pass "EVM signer: $HEALTH_EVM_SIGNER"
        # Auto-populate wallet address for balance check if not provided
        EVM_WALLET="${EVM_WALLET:-$HEALTH_EVM_SIGNER}"
    else
        warn "No EVM signer found in health response" "Check facilitator EVM private key config"
    fi

    if [ -n "$HEALTH_SOL_SIGNER" ]; then
        pass "Solana signer: $HEALTH_SOL_SIGNER"
        SOL_WALLET="${SOL_WALLET:-$HEALTH_SOL_SIGNER}"
    else
        warn "No Solana signer found in health response" "Check facilitator Solana private key config"
    fi

    # Check supported networks/schemes
    NETWORK_COUNT=$(echo "$HEALTH_RESPONSE" | jq '.kinds | length' 2>/dev/null || echo "0")
    info "Supported payment schemes: $NETWORK_COUNT"
else
    fail "Facilitator /health not reachable" \
        "Check: flyctl logs -a $FACILITATOR_APP — or redeploy: cd facilitator && flyctl deploy -a $FACILITATOR_APP"
fi

section "API Health ($API_URL)"

API_HEALTH=$(curl -sf --max-time 10 "$API_URL/health" 2>/dev/null || echo "")

if [ -n "$API_HEALTH" ]; then
    pass "API /health responding"

    X402_STATUS=$(echo "$API_HEALTH" | jq -r '.services.x402 // "unknown"' 2>/dev/null)
    if [ "$X402_STATUS" = "up" ]; then
        pass "API reports x402 facilitator: up"
    elif [ "$X402_STATUS" = "not_configured" ]; then
        fail "API reports x402: not_configured" \
            "flyctl secrets set X402_FACILITATOR_URL=http://$FACILITATOR_APP.internal:8402 -a $API_APP"
    else
        fail "API reports x402 status: $X402_STATUS" \
            "Facilitator not reachable from API internal network. Check both apps are in the same Fly org"
    fi

    OVERALL=$(echo "$API_HEALTH" | jq -r '.status // "unknown"' 2>/dev/null)
    if [ "$OVERALL" = "healthy" ]; then
        pass "API overall: healthy"
    else
        warn "API overall: $OVERALL" "Check: flyctl logs -a $API_APP"
    fi
else
    fail "API /health not reachable at $API_URL" "Check: flyctl logs -a $API_APP"
fi

# API readiness (only if health worked)
if [ -n "$API_HEALTH" ]; then
    API_READY=$(curl -s --max-time 10 -o /dev/null -w "%{http_code}" "$API_URL/health/ready" 2>/dev/null)

    if [ "$API_READY" = "200" ]; then
        pass "API /health/ready: 200 OK"
    elif [ "$API_READY" = "503" ]; then
        READY_BODY=$(curl -s --max-time 10 "$API_URL/health/ready" 2>/dev/null || echo "")
        REASON=$(echo "$READY_BODY" | jq -r '.reason // "unknown"' 2>/dev/null)
        fail "API /health/ready: 503 — $REASON" \
            "Fix $REASON then restart: flyctl machines restart -a $API_APP"
    else
        warn "API /health/ready: HTTP $API_READY" "Unexpected status code"
    fi
fi

# Facilitator /verify and /settle endpoints
if [ -n "$HEALTH_RESPONSE" ]; then
    for endpoint in verify settle; do
        CODE=$(curl -s --max-time 10 -o /dev/null -w "%{http_code}" \
            -X POST "$FACILITATOR_URL/$endpoint" \
            -H "Content-Type: application/json" \
            -d '{}' 2>/dev/null)

        if [ "$CODE" = "000" ]; then
            fail "Facilitator /$endpoint not reachable" "Check facilitator deployment"
        elif [ "$CODE" = "404" ]; then
            fail "Facilitator /$endpoint returned 404" "Wrong facilitator version or misconfigured routes"
        else
            pass "Facilitator /$endpoint exists (HTTP $CODE)"
        fi
    done
fi

# ══════════════════════════════════════════════════════════════════════════════
# 5. RPC Endpoint Connectivity
# ══════════════════════════════════════════════════════════════════════════════

section "RPC Endpoints (public fallbacks)"

info "These test the public default RPCs. Custom RPCs set as Fly secrets can't be tested from here."

# Base RPC
BASE_RPC_RESPONSE=$(curl -sf --max-time 10 -X POST "$BASE_RPC_DEFAULT" \
    -H "Content-Type: application/json" \
    -d '{"jsonrpc":"2.0","method":"eth_chainId","params":[],"id":1}' 2>/dev/null || echo "")

if [ -n "$BASE_RPC_RESPONSE" ]; then
    CHAIN_HEX=$(echo "$BASE_RPC_RESPONSE" | jq -r '.result // ""' 2>/dev/null)
    CHAIN_DEC=$(printf "%d" "$CHAIN_HEX" 2>/dev/null || echo "0")
    if [ "$CHAIN_DEC" -eq "$BASE_CHAIN_ID" ]; then
        pass "Base mainnet RPC OK (chain $CHAIN_DEC)"
    else
        fail "Base RPC returned chain ID $CHAIN_DEC, expected $BASE_CHAIN_ID" "Wrong RPC URL"
    fi
else
    fail "Base mainnet RPC ($BASE_RPC_DEFAULT) unreachable" "Check network connectivity"
fi

# Solana RPC
SOL_RPC_RESPONSE=$(curl -sf --max-time 10 -X POST "$SOLANA_RPC_DEFAULT" \
    -H "Content-Type: application/json" \
    -d '{"jsonrpc":"2.0","id":1,"method":"getHealth"}' 2>/dev/null || echo "")

if [ -n "$SOL_RPC_RESPONSE" ]; then
    SOL_HEALTH=$(echo "$SOL_RPC_RESPONSE" | jq -r '.result // ""' 2>/dev/null)
    if [ "$SOL_HEALTH" = "ok" ]; then
        pass "Solana mainnet RPC OK"
    else
        ERR=$(echo "$SOL_RPC_RESPONSE" | jq -r '.error.message // "unknown"' 2>/dev/null)
        fail "Solana RPC unhealthy: $ERR" "Check Solana network status"
    fi
else
    fail "Solana mainnet RPC ($SOLANA_RPC_DEFAULT) unreachable" "Check network connectivity"
fi

# ══════════════════════════════════════════════════════════════════════════════
# 6. Settlement Wallet Balances
# ══════════════════════════════════════════════════════════════════════════════

section "Settlement Wallet Balances"

if [ -z "$EVM_WALLET" ]; then
    fail "No EVM wallet address available — facilitator health didn't return a signer" \
        "Re-run with: FACILITATOR_EVM_ADDRESS=0x... $0"
fi

if [ -z "$SOL_WALLET" ]; then
    fail "No Solana wallet address available — facilitator health didn't return a signer" \
        "Re-run with: FACILITATOR_SOLANA_ADDRESS=... $0"
fi

# EVM: ETH balance on Base
if [ -n "$EVM_WALLET" ]; then
    ETH_RESP=$(curl -sf --max-time 10 -X POST "$BASE_RPC_DEFAULT" \
        -H "Content-Type: application/json" \
        -d "{\"jsonrpc\":\"2.0\",\"method\":\"eth_getBalance\",\"params\":[\"$EVM_WALLET\",\"latest\"],\"id\":1}" \
        2>/dev/null || echo "")

    if [ -n "$ETH_RESP" ]; then
        ETH_HEX=$(echo "$ETH_RESP" | jq -r '.result // "0x0"' 2>/dev/null)
        ETH_WEI=$(printf "%d" "$ETH_HEX" 2>/dev/null || echo "0")

        if command -v bc &>/dev/null; then
            ETH_DISPLAY=$(echo "scale=6; $ETH_WEI / 1000000000000000000" | bc 2>/dev/null || echo "?")
        else
            ETH_DISPLAY="$ETH_WEI wei"
        fi

        if [ "$ETH_WEI" -gt "$MIN_ETH_WEI" ] 2>/dev/null; then
            pass "EVM wallet ($EVM_WALLET): ${ETH_DISPLAY} ETH"
        elif [ "$ETH_WEI" -eq 0 ] 2>/dev/null; then
            fail "EVM wallet ($EVM_WALLET): ZERO ETH" \
                "Fund with ETH on Base for gas — even 0.01 ETH covers ~90k settlements"
        else
            warn "EVM wallet ($EVM_WALLET): low balance ${ETH_DISPLAY} ETH" \
                "Top up with more ETH on Base"
        fi
    else
        fail "Could not query EVM balance" "Base RPC issue"
    fi
fi

# Solana: SOL balance
if [ -n "$SOL_WALLET" ]; then
    SOL_RESP=$(curl -sf --max-time 10 -X POST "$SOLANA_RPC_DEFAULT" \
        -H "Content-Type: application/json" \
        -d "{\"jsonrpc\":\"2.0\",\"id\":1,\"method\":\"getBalance\",\"params\":[\"$SOL_WALLET\"]}" \
        2>/dev/null || echo "")

    if [ -n "$SOL_RESP" ]; then
        LAMPORTS=$(echo "$SOL_RESP" | jq -r '.result.value // 0' 2>/dev/null)

        if command -v bc &>/dev/null; then
            SOL_DISPLAY=$(echo "scale=6; $LAMPORTS / 1000000000" | bc 2>/dev/null || echo "?")
        else
            SOL_DISPLAY="$LAMPORTS lamports"
        fi

        if [ "$LAMPORTS" -gt "$MIN_SOL_LAMPORTS" ] 2>/dev/null; then
            pass "Solana wallet ($SOL_WALLET): ${SOL_DISPLAY} SOL"
        elif [ "$LAMPORTS" -eq 0 ] 2>/dev/null; then
            fail "Solana wallet ($SOL_WALLET): ZERO SOL" \
                "Fund with SOL for tx fees — 0.1 SOL covers thousands of settlements"
        else
            warn "Solana wallet ($SOL_WALLET): low balance ${SOL_DISPLAY} SOL" \
                "Top up with more SOL"
        fi
    else
        fail "Could not query Solana balance" "Solana RPC issue"
    fi
fi

# ══════════════════════════════════════════════════════════════════════════════
# 7. USDC Token Contracts
# ══════════════════════════════════════════════════════════════════════════════

section "USDC Token Contracts"

# Base USDC — call symbol()
SYMBOL_RESP=$(curl -sf --max-time 10 -X POST "$BASE_RPC_DEFAULT" \
    -H "Content-Type: application/json" \
    -d "{\"jsonrpc\":\"2.0\",\"method\":\"eth_call\",\"params\":[{\"to\":\"$BASE_USDC\",\"data\":\"0x95d89b41\"},\"latest\"],\"id\":1}" \
    2>/dev/null || echo "")

if [ -n "$SYMBOL_RESP" ]; then
    SYM=$(echo "$SYMBOL_RESP" | jq -r '.result // ""' 2>/dev/null)
    if [ -n "$SYM" ] && [ "$SYM" != "null" ] && [ "$SYM" != "0x" ]; then
        pass "Base USDC contract ($BASE_USDC) valid"
    else
        fail "Base USDC contract returned empty" "Check address in facilitator/config.json"
    fi
else
    fail "Could not query Base USDC contract" "Base RPC issue"
fi

# Solana USDC — check mint account owner
MINT_RESP=$(curl -sf --max-time 10 -X POST "$SOLANA_RPC_DEFAULT" \
    -H "Content-Type: application/json" \
    -d "{\"jsonrpc\":\"2.0\",\"id\":1,\"method\":\"getAccountInfo\",\"params\":[\"$SOLANA_USDC_MINT\",{\"encoding\":\"jsonParsed\"}]}" \
    2>/dev/null || echo "")

if [ -n "$MINT_RESP" ]; then
    OWNER=$(echo "$MINT_RESP" | jq -r '.result.value.owner // ""' 2>/dev/null)
    if [ "$OWNER" = "TokenkegQfeZyiNwAJbNbGKPFXCWuBvf9Ss623VQ5DA" ]; then
        pass "Solana USDC mint ($SOLANA_USDC_MINT) valid"
    elif [ -n "$OWNER" ] && [ "$OWNER" != "null" ]; then
        warn "Solana USDC mint owner: $OWNER (expected SPL Token program)" "Verify mint address"
    else
        fail "Solana USDC mint not found" "Check address in facilitator/config.json"
    fi
else
    fail "Could not query Solana USDC mint" "Solana RPC issue"
fi

# ══════════════════════════════════════════════════════════════════════════════
# Summary
# ══════════════════════════════════════════════════════════════════════════════

echo ""
echo -e "${BOLD}────────────────────────────────────────────────────────────${NC}"
echo -e "${BOLD}Summary${NC}"
echo ""
echo -e "  ${GREEN}Passed:${NC}  $PASS_COUNT"
echo -e "  ${RED}Failed:${NC}  $FAIL_COUNT"
echo -e "  ${YELLOW}Warnings:${NC} $WARN_COUNT"
echo ""

if [ ${#ACTIONS[@]} -gt 0 ]; then
    echo -e "${BOLD}${RED}Action items:${NC}"
    echo ""
    for i in "${!ACTIONS[@]}"; do
        echo -e "  $((i + 1)). ${ACTIONS[$i]}"
    done
    echo ""
fi

if [ "$FAIL_COUNT" -eq 0 ] && [ "$WARN_COUNT" -eq 0 ]; then
    echo -e "${GREEN}${BOLD}All checks passed — facilitator infrastructure is ready!${NC}"
    echo ""
    exit 0
elif [ "$FAIL_COUNT" -eq 0 ]; then
    echo -e "${YELLOW}${BOLD}No failures, but $WARN_COUNT warning(s) to review.${NC}"
    echo ""
    exit 0
else
    echo -e "${RED}${BOLD}$FAIL_COUNT check(s) failed — fix the items above.${NC}"
    echo ""
    exit 1
fi
