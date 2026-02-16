# Test: stronghold wallet export
# Verifies wallet export creates backup files.

test_start "Wallet export — creates backup files"
EXPORT_DIR="/tmp/test-wallet-backup"
rm -f "${EXPORT_DIR}" "${EXPORT_DIR}-solana"

# Pipe "y" for the confirmation prompt
echo "y" | stronghold wallet export --output "$EXPORT_DIR" > /tmp/export_output.txt 2>&1
EXPORT_EXIT=$?
EXPORT_OUTPUT=$(cat /tmp/export_output.txt)
rm -f /tmp/export_output.txt

if [ "$EXPORT_EXIT" -eq 0 ]; then
    assert_file_exists "$EXPORT_DIR" "EVM backup file created"
    assert_contains "$EXPORT_OUTPUT" "wallet exported" "export confirms success"

    # Check Solana backup too
    if [ -f "${EXPORT_DIR}-solana" ]; then
        test_pass "Solana backup file created"
    else
        test_skip "Solana backup not created (may not have Solana wallet in keyring)"
    fi
else
    test_fail "wallet export failed (exit $EXPORT_EXIT): ${EXPORT_OUTPUT:0:200}"
fi

# Cleanup
rm -f "${EXPORT_DIR}" "${EXPORT_DIR}-solana"
