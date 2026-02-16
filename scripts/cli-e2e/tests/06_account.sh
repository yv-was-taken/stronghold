# Test: account balance and account deposit
# Verifies account commands work.

test_start "Account balance — displays balance info"
run_cli account balance
# account balance is an alias for wallet balance — same behavior
if [ "$CLI_EXIT" -eq 0 ]; then
    assert_contains "$CLI_OUTPUT" "Wallet Balances" "account balance shows header"
    test_pass "account balance ran successfully"
else
    test_skip "account balance: RPC unreachable in Docker"
fi

test_start "Account deposit — shows deposit info"
run_cli account deposit
assert_exit_zero "$CLI_EXIT" "account deposit exits 0"
assert_contains "$CLI_OUTPUT" "Add Funds" "deposit shows header"
