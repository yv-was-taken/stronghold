# Test: Prerequisites
# Verify the test environment is functional.

test_start "Prerequisites — API health"
API_STATUS=$(curl -s -o /dev/null -w "%{http_code}" "$STRONGHOLD_API_URL/health")
assert_exit_code 200 "$API_STATUS" "API health endpoint returns 200"

test_start "Prerequisites — CLI binary"
if command -v stronghold &> /dev/null; then
    test_pass "stronghold binary found in PATH"
else
    test_fail "stronghold binary not found in PATH"
fi

test_start "Prerequisites — CLI responds"
run_cli --version
assert_exit_zero "$CLI_EXIT" "stronghold --version exits 0"
assert_contains "$CLI_OUTPUT" "stronghold" "version output contains 'stronghold'"
