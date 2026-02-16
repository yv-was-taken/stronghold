# Test: stronghold health
# Verifies health command shows API status and RPC info.

test_start "Health — shows API status"
run_cli health
# Health command itself should exit 0 even if some services are down
assert_exit_zero "$CLI_EXIT" "health exits 0"
assert_contains "$CLI_OUTPUT" "Stronghold Health" "health shows header"
assert_contains "$CLI_OUTPUT" "API:" "health shows API section"
