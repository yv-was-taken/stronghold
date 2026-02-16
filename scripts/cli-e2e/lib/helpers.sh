#!/bin/bash
# CLI E2E Test Framework
# Provides assertion helpers, test counters, and well-known test keys.

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

# Counters
TESTS_PASSED=0
TESTS_FAILED=0
TESTS_SKIPPED=0
CURRENT_TEST=""

# Well-known test keys (Hardhat account #0 for EVM, deterministic Ed25519 for Solana)
# These are PUBLIC test keys - never use on mainnet.
TEST_EVM_PRIVATE_KEY="ac0974bec39a17e36ba4a6b4d238ff944bacb478cbed5efcae784d7bf4f2ff80"
TEST_EVM_ADDRESS="0xf39Fd6e51aad88F6F4ce6aB8827279cffFb92266"
TEST_SOLANA_PRIVATE_KEY="4wBqpZM9xaSheZzJSMYMnaxHimMpQPKMaFsKk3W1MhpDjVCMLxf2Lbsfi3VZBpKQDavZKiV3FkMR24GYG23KXjpS"
TEST_SOLANA_ADDRESS="CJsLwbP1iu5DuUikHEJnLfANgKy6stB2uFgvBBHoyxGz"

# Replacement keys for wallet replace tests
REPLACE_EVM_PRIVATE_KEY="59c6995e998f97a5a0044966f0945389dc9e86dae88c7a8412f4603b6b78690d"
REPLACE_EVM_ADDRESS="0x70997970C51812dc3A010C7d01b50e0d17dc79C8"
REPLACE_SOLANA_PRIVATE_KEY="3Mc1cnVrS4BNi3mGWrJMnFJLPYRjVJcvezqEkPkFbDGRYDPFyBbaMgGkCm7MbFNYBtQxTNPbodPN2m9ipRfGSEZp"
REPLACE_SOLANA_ADDRESS="J5n3ND3oKy7MHFfYVUJchbfJhRHUFaHpHPABaKLzZib5"

# Shared state between tests (set by 03_init_create.sh, used by 12_init_login.sh)
SAVED_ACCOUNT_NUMBER=""

# --- Test lifecycle ---

test_start() {
    CURRENT_TEST="$1"
    echo ""
    echo -e "${BLUE}TEST: $1${NC}"
}

test_pass() {
    echo -e "${GREEN}  ✓ PASS: $1${NC}"
    TESTS_PASSED=$((TESTS_PASSED + 1))
}

test_fail() {
    echo -e "${RED}  ✗ FAIL: $1${NC}"
    TESTS_FAILED=$((TESTS_FAILED + 1))
}

test_skip() {
    echo -e "${YELLOW}  ⊘ SKIP: $1${NC}"
    TESTS_SKIPPED=$((TESTS_SKIPPED + 1))
}

# --- Assertions ---

assert_exit_code() {
    local expected=$1
    local actual=$2
    local msg=$3
    if [ "$expected" -eq "$actual" ]; then
        test_pass "$msg"
    else
        test_fail "$msg (expected exit code $expected, got $actual)"
    fi
}

assert_exit_zero() {
    local actual=$1
    local msg=$2
    assert_exit_code 0 "$actual" "$msg"
}

assert_contains() {
    local haystack="$1"
    local needle="$2"
    local msg="$3"
    if echo "$haystack" | grep -q "$needle"; then
        test_pass "$msg"
    else
        test_fail "$msg (expected to contain '$needle')"
        echo "    Output was: ${haystack:0:300}"
    fi
}

assert_not_contains() {
    local haystack="$1"
    local needle="$2"
    local msg="$3"
    if echo "$haystack" | grep -q "$needle"; then
        test_fail "$msg (should NOT contain '$needle')"
    else
        test_pass "$msg"
    fi
}

assert_file_exists() {
    local path="$1"
    local msg="$2"
    if [ -f "$path" ]; then
        test_pass "$msg"
    else
        test_fail "$msg (file not found: $path)"
    fi
}

assert_file_contains() {
    local path="$1"
    local needle="$2"
    local msg="$3"
    if [ -f "$path" ] && grep -q "$needle" "$path"; then
        test_pass "$msg"
    else
        test_fail "$msg (file $path does not contain '$needle')"
    fi
}

# --- CLI runner ---

# Run CLI capturing output, preserving real exit code
CLI_OUTPUT=""
CLI_EXIT=0

run_cli() {
    local tmpfile
    tmpfile=$(mktemp)
    set +e
    stronghold "$@" > "$tmpfile" 2>&1
    CLI_EXIT=$?
    set -e
    CLI_OUTPUT=$(cat "$tmpfile")
    rm -f "$tmpfile"
}

# --- Results ---

report_results() {
    echo ""
    echo -e "${BLUE}═══════════════════════════════════════════════════════════════${NC}"
    echo -e "${BLUE}                    TEST RESULTS SUMMARY                        ${NC}"
    echo -e "${BLUE}═══════════════════════════════════════════════════════════════${NC}"
    echo ""
    echo -e "  Tests passed:  ${GREEN}$TESTS_PASSED${NC}"
    echo -e "  Tests failed:  ${RED}$TESTS_FAILED${NC}"
    echo -e "  Tests skipped: ${YELLOW}$TESTS_SKIPPED${NC}"
    echo ""

    if [ "$TESTS_FAILED" -gt 0 ]; then
        echo -e "${RED}Some tests failed!${NC}"
        exit 1
    else
        echo -e "${GREEN}All CLI E2E tests passed!${NC}"
        exit 0
    fi
}
