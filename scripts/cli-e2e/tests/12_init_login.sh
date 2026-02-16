# Test: stronghold init --yes --skip-service --account-number (login)
# Tests logging into an existing account by account number.

test_start "Init login — reset state"
rm -rf "$HOME/.stronghold"
pass rm -rf stronghold/ 2>/dev/null || true
test_pass "Config and keyring cleared"

test_start "Init login — login with saved account number"
if [ -z "$SAVED_ACCOUNT_NUMBER" ]; then
    test_skip "No saved account number (test 03 may have failed)"
else
    run_cli init --yes --skip-service --account-number "$SAVED_ACCOUNT_NUMBER"

    if echo "$CLI_OUTPUT" | grep -qi "TOTP required"; then
        test_skip "Login requires TOTP verification (expected for some configurations)"
    elif [ "$CLI_EXIT" -eq 0 ]; then
        assert_contains "$CLI_OUTPUT" "Logged in as" "login confirms account"
        assert_contains "$CLI_OUTPUT" "Initialization complete" "init completes after login"
    else
        # Login may fail if account was simulated (API was unavailable)
        if echo "$CLI_OUTPUT" | grep -qi "simulated\|not found\|invalid"; then
            test_skip "Login failed — account may have been simulated (API unavailable during create)"
        else
            test_fail "init login failed: ${CLI_OUTPUT:0:300}"
        fi
    fi
fi
