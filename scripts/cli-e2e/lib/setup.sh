#!/bin/bash
# CLI E2E Test Setup
# Validates environment and ensures GPG/pass keyring is initialized.

setup_test_environment() {
    echo -e "${BLUE}╔════════════════════════════════════════════════════════════╗${NC}"
    echo -e "${BLUE}║     Stronghold CLI End-to-End Tests                        ║${NC}"
    echo -e "${BLUE}╚════════════════════════════════════════════════════════════╝${NC}"
    echo ""

    # Validate API URL
    if [ -z "$STRONGHOLD_API_URL" ]; then
        echo -e "${RED}ERROR: STRONGHOLD_API_URL not set${NC}"
        exit 1
    fi
    echo "API URL: $STRONGHOLD_API_URL"

    # Validate stronghold binary
    if ! command -v stronghold &> /dev/null; then
        echo -e "${RED}ERROR: stronghold binary not found in PATH${NC}"
        exit 1
    fi
    echo "CLI binary: $(which stronghold)"

    # Validate GPG/pass keyring (Dockerfile.cli-test entrypoint initializes this)
    if ! command -v pass &> /dev/null; then
        echo -e "${RED}ERROR: pass not installed${NC}"
        exit 1
    fi

    # Verify pass is initialized
    if ! pass ls > /dev/null 2>&1; then
        echo -e "${YELLOW}Initializing pass keyring...${NC}"
        # Generate GPG key if needed
        if ! gpg --list-keys "Stronghold Test" > /dev/null 2>&1; then
            gpg --batch --passphrase "" --quick-gen-key "Stronghold Test <test@stronghold.local>" default default never 2>/dev/null
        fi
        pass init "Stronghold Test" 2>/dev/null
    fi
    echo "Keyring: pass (GPG initialized)"

    # Clean any prior test state
    rm -rf ~/.stronghold

    echo ""
}
