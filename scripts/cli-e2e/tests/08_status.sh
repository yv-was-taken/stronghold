# Test: stronghold status
# Verifies status command runs without crashing.

test_start "Status — runs without error"
run_cli status
assert_exit_zero "$CLI_EXIT" "status exits 0"
assert_contains "$CLI_OUTPUT" "Stronghold Status" "status shows header"
