# Test: stronghold wallet list
# Verifies wallet list shows both EVM and Solana wallets.

test_start "Wallet list — shows both chains"
run_cli wallet list
assert_exit_zero "$CLI_EXIT" "wallet list exits 0"
assert_contains "$CLI_OUTPUT" "EVM (Base):" "shows EVM section"
assert_contains "$CLI_OUTPUT" "0x" "shows EVM address"
assert_contains "$CLI_OUTPUT" "Solana:" "shows Solana section"
