# Test: stronghold doctor
# Verifies system prerequisites check runs and produces expected output.

test_start "Doctor — runs successfully"
run_cli doctor
# Doctor may exit non-zero if checks fail (e.g., no root), that's OK in container
assert_contains "$CLI_OUTPUT" "Stronghold Doctor" "doctor shows header"
assert_contains "$CLI_OUTPUT" "Operating System" "doctor checks OS"
assert_contains "$CLI_OUTPUT" "Summary:" "doctor shows summary"
