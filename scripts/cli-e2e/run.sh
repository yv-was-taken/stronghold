#!/bin/bash
# CLI E2E Test Orchestrator
# Sources the test framework and runs all test files in order.

set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# Source test framework
source "$SCRIPT_DIR/lib/helpers.sh"
source "$SCRIPT_DIR/lib/setup.sh"

# Initialize test environment
setup_test_environment

# Run test files in order
for test_file in "$SCRIPT_DIR"/tests/[0-9]*.sh; do
    if [ -f "$test_file" ]; then
        echo ""
        echo -e "${BLUE}━━━ $(basename "$test_file") ━━━${NC}"
        source "$test_file"
    fi
done

# Print results and exit
report_results
