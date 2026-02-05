#!/bin/bash
# Stronghold x402 End-to-End Test
#
# This script tests the FULL x402 payment flow with real testnet funds.
# It requires a wallet funded with Base Sepolia USDC.
#
# Prerequisites:
#   1. Docker and Docker Compose installed
#   2. TEST_PRIVATE_KEY - Private key of a wallet with Base Sepolia USDC
#   3. At least 0.01 USDC on Base Sepolia (enough for ~5 test scans)
#
# Getting testnet tokens:
#   1. Base Sepolia ETH: https://www.alchemy.com/faucets/base-sepolia
#   2. Base Sepolia USDC: https://faucet.circle.com/ (select Base Sepolia)
#
# Usage:
#   export TEST_PRIVATE_KEY=0x...
#   ./scripts/e2e-x402-test.sh

set -e

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(dirname "$SCRIPT_DIR")"

echo -e "${BLUE}╔════════════════════════════════════════════════════════════╗${NC}"
echo -e "${BLUE}║     Stronghold x402 End-to-End Test                        ║${NC}"
echo -e "${BLUE}║     Testing REAL payments on Base Sepolia testnet          ║${NC}"
echo -e "${BLUE}╚════════════════════════════════════════════════════════════╝${NC}"
echo ""

# ==================== Prerequisites Check ====================

echo -e "${YELLOW}Checking prerequisites...${NC}"

# Check Docker
if ! command -v docker &> /dev/null; then
    echo -e "${RED}Error: Docker is not installed${NC}"
    exit 1
fi
echo -e "${GREEN}✓ Docker installed${NC}"

# Check Docker Compose
if ! command -v docker-compose &> /dev/null && ! docker compose version &> /dev/null 2>&1; then
    echo -e "${RED}Error: Docker Compose is not installed${NC}"
    exit 1
fi
echo -e "${GREEN}✓ Docker Compose installed${NC}"

# Check TEST_PRIVATE_KEY
if [ -z "$TEST_PRIVATE_KEY" ]; then
    echo ""
    echo -e "${RED}Error: TEST_PRIVATE_KEY environment variable is required${NC}"
    echo ""
    echo "This e2e test requires REAL testnet funds to verify x402 payments work."
    echo ""
    echo -e "${BLUE}To get testnet tokens:${NC}"
    echo "1. Create a wallet and export the private key"
    echo "2. Get Base Sepolia ETH: https://www.alchemy.com/faucets/base-sepolia"
    echo "3. Get Base Sepolia USDC: https://faucet.circle.com/ (select Base Sepolia)"
    echo "   USDC Contract: 0x036CbD53842c5426634e7929541eC2318f3dCF7e"
    echo ""
    echo "4. Export and run:"
    echo "   export TEST_PRIVATE_KEY=0x..."
    echo "   ./scripts/e2e-x402-test.sh"
    echo ""
    exit 1
fi
echo -e "${GREEN}✓ TEST_PRIVATE_KEY provided${NC}"

# Derive wallet address from private key for API payments
# We'll use the same wallet for both client and API receiver in test mode
if [ -z "$API_WALLET_ADDRESS" ]; then
    # Default to a test receiver address - in real test you'd set this
    export API_WALLET_ADDRESS="0x0000000000000000000000000000000000000001"
    echo -e "${YELLOW}! API_WALLET_ADDRESS not set, using test address${NC}"
fi

echo ""

# ==================== Cleanup Function ====================

cleanup() {
    echo ""
    echo -e "${YELLOW}Cleaning up...${NC}"
    cd "$PROJECT_DIR"
    docker-compose -f docker-compose.e2e.yml down -v --remove-orphans 2>/dev/null || true
    echo -e "${GREEN}✓ Cleanup complete${NC}"
}
trap cleanup EXIT

# ==================== Start Services ====================

echo -e "${BLUE}Building and starting services...${NC}"
cd "$PROJECT_DIR"

# Build fresh images
docker-compose -f docker-compose.e2e.yml build --quiet

# Start postgres and api first
docker-compose -f docker-compose.e2e.yml up -d postgres
echo "Waiting for PostgreSQL..."
sleep 5

docker-compose -f docker-compose.e2e.yml up -d api
echo "Waiting for API..."

# Wait for API to be healthy
for i in {1..60}; do
    if curl -s http://localhost:8080/health > /dev/null 2>&1; then
        echo -e "${GREEN}✓ API is ready${NC}"
        break
    fi
    if [ $i -eq 60 ]; then
        echo -e "${RED}Error: API failed to start${NC}"
        docker-compose -f docker-compose.e2e.yml logs api
        exit 1
    fi
    sleep 1
done

echo ""

# ==================== Run E2E Tests ====================

echo -e "${BLUE}Running e2e tests...${NC}"
echo ""

# Run the test script inside the CLI container
docker-compose -f docker-compose.e2e.yml run --rm cli /app/scripts/e2e-tests-internal.sh
TEST_EXIT_CODE=$?

echo ""

if [ $TEST_EXIT_CODE -eq 0 ]; then
    echo -e "${GREEN}╔════════════════════════════════════════════════════════════╗${NC}"
    echo -e "${GREEN}║     All x402 e2e tests PASSED!                             ║${NC}"
    echo -e "${GREEN}╚════════════════════════════════════════════════════════════╝${NC}"
else
    echo -e "${RED}╔════════════════════════════════════════════════════════════╗${NC}"
    echo -e "${RED}║     x402 e2e tests FAILED                                  ║${NC}"
    echo -e "${RED}╚════════════════════════════════════════════════════════════╝${NC}"
    exit 1
fi
