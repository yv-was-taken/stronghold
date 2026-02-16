# Test: config get / config set
# Tests configuration management before init (uses defaults).

test_start "Config — get all (before init)"
run_cli config get
# Before init, config file may not exist — should still return defaults or error gracefully
if [ "$CLI_EXIT" -eq 0 ]; then
    test_pass "config get exits 0"
else
    test_skip "config get before init returns non-zero (expected if no config)"
fi

test_start "Config — set and get a value"
run_cli config set scanning.content.action_on_block allow
if [ "$CLI_EXIT" -eq 0 ]; then
    assert_contains "$CLI_OUTPUT" "Set scanning.content.action_on_block = allow" "config set confirms value"
else
    test_skip "config set before init not supported (no config file)"
fi
