# Test: stronghold init --yes --skip-service (create new account)
# Creates a new account with auto-generated dual wallets.
# State from this test is used by tests 04-10.

test_start "Init — create new account with dual wallets"
run_cli init --yes --skip-service
assert_exit_zero "$CLI_EXIT" "init --yes --skip-service exits 0"
assert_contains "$CLI_OUTPUT" "EVM wallet created: 0x" "EVM wallet created"
assert_contains "$CLI_OUTPUT" "Solana wallet created:" "Solana wallet created"
assert_contains "$CLI_OUTPUT" "Account:" "account number shown"
assert_contains "$CLI_OUTPUT" "Initialization complete" "init completes"

test_start "Init — config file created"
assert_file_exists "$HOME/.stronghold/config.yaml" "config.yaml exists"
assert_file_contains "$HOME/.stronghold/config.yaml" "address:" "config has EVM address"
assert_file_contains "$HOME/.stronghold/config.yaml" "solana_address:" "config has Solana address"
assert_file_contains "$HOME/.stronghold/config.yaml" "account_number:" "config has account number"

# Save account number for test 12 (init --account-number login)
SAVED_ACCOUNT_NUMBER=$(grep 'account_number:' "$HOME/.stronghold/config.yaml" | awk '{print $2}' | tr -d '"')
if [ -n "$SAVED_ACCOUNT_NUMBER" ]; then
    test_pass "Account number saved: $SAVED_ACCOUNT_NUMBER"
    export SAVED_ACCOUNT_NUMBER
else
    test_fail "Could not extract account number from config"
fi
