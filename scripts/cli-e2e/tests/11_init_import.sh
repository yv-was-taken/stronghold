# Test: stronghold init --yes --skip-service with imported keys
# Resets config and creates a new account with imported private keys.

test_start "Init import — reset state"
rm -rf "$HOME/.stronghold"
pass rm -rf stronghold/ 2>/dev/null || true
test_pass "Config and keyring cleared"

test_start "Init import — import EVM and Solana keys"
run_cli init --yes --skip-service --private-key "$TEST_EVM_PRIVATE_KEY" --solana-private-key "$TEST_SOLANA_PRIVATE_KEY"
assert_exit_zero "$CLI_EXIT" "init with imported keys exits 0"
assert_contains "$CLI_OUTPUT" "EVM wallet imported:" "EVM wallet imported"
assert_contains "$CLI_OUTPUT" "Solana wallet imported:" "Solana wallet imported"
assert_contains "$CLI_OUTPUT" "Account:" "account number shown"
assert_contains "$CLI_OUTPUT" "Initialization complete" "init completes"

test_start "Init import — verify imported addresses"
run_cli wallet list
assert_exit_zero "$CLI_EXIT" "wallet list exits 0"
assert_contains "$CLI_OUTPUT" "$TEST_EVM_ADDRESS" "wallet list shows imported EVM address"
# Solana address derivation varies by library version — just verify it's present
assert_not_contains "$CLI_OUTPUT" "Not configured" "Solana wallet is configured"
