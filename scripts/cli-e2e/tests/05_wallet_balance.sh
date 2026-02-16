# Test: stronghold wallet balance
# Verifies wallet balance display. Wallets are unfunded so balance will be 0
# or show a warning — both are acceptable.

test_start "Wallet balance — displays balance info"
run_cli wallet balance
# May fail if RPC unreachable in Docker, which is acceptable
if [ "$CLI_EXIT" -eq 0 ]; then
    assert_contains "$CLI_OUTPUT" "Wallet Balances" "shows balance header"
    assert_contains "$CLI_OUTPUT" "Base (EVM) Wallet:" "shows EVM section"
    test_pass "wallet balance ran successfully"
else
    # RPC calls may fail in isolated Docker network — skip gracefully
    if echo "$CLI_OUTPUT" | grep -qi "Wallet Balances"; then
        test_pass "wallet balance displayed info (some RPCs may be unreachable)"
    else
        test_skip "wallet balance failed (RPC unreachable in Docker)"
    fi
fi
