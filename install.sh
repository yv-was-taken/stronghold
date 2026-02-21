#!/bin/bash
#
# Stronghold CLI Installer
# Usage: curl -fsSL https://getstronghold.xyz/install.sh | sh
#

set -e

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Configuration
REPO_URL="https://github.com/yv-was-taken/stronghold"
INSTALL_DIR="/usr/local/bin"
VERSION="${VERSION:-latest}"

# Print functions
print_info() {
    echo -e "${BLUE}ℹ${NC} $1"
}

print_success() {
    echo -e "${GREEN}✓${NC} $1"
}

print_warning() {
    echo -e "${YELLOW}⚠${NC} $1"
}

print_error() {
    echo -e "${RED}✗${NC} $1"
}

# Detect OS and architecture
detect_platform() {
    OS=$(uname -s | tr '[:upper:]' '[:lower:]')
    ARCH=$(uname -m)

    case "$OS" in
        linux)
            PLATFORM="linux"
            ;;
        darwin)
            PLATFORM="darwin"
            ;;
        *)
            print_error "Unsupported operating system: $OS"
            print_info "Stronghold supports Linux and macOS only."
            exit 1
            ;;
    esac

    case "$ARCH" in
        x86_64|amd64)
            ARCH="amd64"
            ;;
        arm64|aarch64)
            ARCH="arm64"
            ;;
        *)
            print_error "Unsupported architecture: $ARCH"
            exit 1
            ;;
    esac

    print_success "Detected platform: $PLATFORM/$ARCH"
}

# Check dependencies
check_dependencies() {
    print_info "Checking dependencies..."

    if ! command -v curl &> /dev/null; then
        print_error "curl is required but not installed"
        exit 1
    fi

    print_success "All dependencies satisfied"
}

# Detect Linux distribution
detect_distro() {
    if [[ "$OSTYPE" != "linux-gnu"* ]]; then
        return
    fi

    if [ -f /etc/os-release ]; then
        . /etc/os-release
        DISTRO=$ID
        DISTRO_NAME=$NAME
    elif command -v lsb_release &> /dev/null; then
        DISTRO=$(lsb_release -si | tr '[:upper:]' '[:lower:]')
        DISTRO_NAME=$(lsb_release -sd)
    else
        DISTRO="unknown"
        DISTRO_NAME="Unknown Linux"
    fi

    print_info "Detected distribution: $DISTRO_NAME"
}

# Check if a keyring backend is available
check_keyring_available() {
    # Check for Secret Service (GNOME Keyring, KWallet5+)
    if command -v secret-tool &> /dev/null; then
        return 0
    fi
    # Check for KWallet
    if command -v kwalletd5 &> /dev/null || command -v kwalletd6 &> /dev/null; then
        return 0
    fi
    # Check for pass
    if command -v pass &> /dev/null; then
        return 0
    fi
    return 1
}

# Show manual installation instructions for keyring
show_manual_keyring_instructions() {
    echo
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo "Manual Keyring Installation Instructions"
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo
    echo "Stronghold requires a secure keyring to store your wallet"
    echo "private keys. The following options are supported:"
    echo
    echo "1. GNOME Keyring / Secret Service (recommended for most users)"
    echo "   Debian/Ubuntu:    sudo apt-get install gnome-keyring"
    echo "   RHEL/CentOS:      sudo yum install gnome-keyring"
    echo "   Fedora:           sudo dnf install gnome-keyring"
    echo "   Arch:             sudo pacman -S gnome-keyring"
    echo "   openSUSE:         sudo zypper install gnome-keyring"
    echo
    echo "2. KWallet (for KDE Plasma users)"
    echo "   Usually pre-installed with KDE Plasma"
    echo "   Debian/Ubuntu:    sudo apt-get install kwalletmanager"
    echo
    echo "3. pass (password-store, works on headless servers)"
    echo "   Debian/Ubuntu:    sudo apt-get install pass"
    echo "   RHEL/CentOS:      sudo yum install pass"
    echo "   Fedora:           sudo dnf install pass"
    echo "   Arch:             sudo pacman -S pass"
    echo "   Then initialize:  gpg --gen-key && pass init <your-email>"
    echo
    echo "After installing one of the above, re-run this installer."
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo
}

# Install keyring dependencies for Linux
install_keyring_deps() {
    if [[ "$OSTYPE" != "linux-gnu"* ]]; then
        # macOS and Windows have built-in keyrings
        return 0
    fi

    print_info "Checking for secure keyring..."

    if check_keyring_available; then
        print_success "Secure keyring is available"
        return 0
    fi

    echo
    print_warning "No secure keyring detected on your system"
    echo
    echo "Stronghold uses your operating system's keyring to securely"
    echo "store wallet private keys. We detected that you don't have"
    echo "one of the supported keyrings installed."
    echo

    # Check if we can auto-install
    if ! command -v sudo &> /dev/null; then
        print_warning "sudo is not available - cannot auto-install dependencies"
        show_manual_keyring_instructions
        exit 1
    fi

    if [ "$EUID" -ne 0 ] && ! sudo -n true 2>/dev/null; then
        print_warning "Root access required to install dependencies"
        print_info "You will be prompted for your password"
        echo
    fi

    # Try to auto-install based on distro
    detect_distro

    echo "Would you like us to install the required dependencies?"
    echo "  - gnome-keyring (secure key storage)"
    echo "  - dbus-x11 (required for keyring communication)"
    echo
    printf "Install automatically? [Y/n] "
    read -r response
    response=${response:-Y}

    if [[ ! "$response" =~ ^[Yy]$ ]]; then
        show_manual_keyring_instructions
        exit 1
    fi

    print_info "Installing keyring dependencies..."

    case "$DISTRO" in
        ubuntu|debian)
            echo "  → Running: sudo apt-get update"
            if ! sudo apt-get update -qq; then
                print_error "Failed to update package lists"
                show_manual_keyring_instructions
                exit 1
            fi
            echo "  → Running: sudo apt-get install -y gnome-keyring dbus-x11"
            if ! sudo apt-get install -y gnome-keyring dbus-x11; then
                print_error "Failed to install packages"
                show_manual_keyring_instructions
                exit 1
            fi
            ;;

        fedora)
            echo "  → Running: sudo dnf install -y gnome-keyring dbus-x11"
            if ! sudo dnf install -y gnome-keyring dbus-x11; then
                print_error "Failed to install packages"
                show_manual_keyring_instructions
                exit 1
            fi
            ;;

        rhel|centos|almalinux|rocky)
            echo "  → Running: sudo yum install -y gnome-keyring dbus-x11"
            if ! sudo yum install -y gnome-keyring dbus-x11; then
                print_error "Failed to install packages"
                show_manual_keyring_instructions
                exit 1
            fi
            ;;

        arch|manjaro)
            echo "  → Running: sudo pacman -S --noconfirm gnome-keyring dbus"
            if ! sudo pacman -S --noconfirm gnome-keyring dbus; then
                print_error "Failed to install packages"
                show_manual_keyring_instructions
                exit 1
            fi
            ;;

        opensuse*)
            echo "  → Running: sudo zypper install -y gnome-keyring dbus-1-x11"
            if ! sudo zypper install -y gnome-keyring dbus-1-x11; then
                print_error "Failed to install packages"
                show_manual_keyring_instructions
                exit 1
            fi
            ;;

        alpine)
            echo "  → Running: sudo apk add gnome-keyring dbus"
            if ! sudo apk add gnome-keyring dbus; then
                print_error "Failed to install packages"
                show_manual_keyring_instructions
                exit 1
            fi
            ;;

        *)
            print_error "Unsupported distribution: $DISTRO_NAME"
            echo
            echo "We couldn't automatically install dependencies for your distribution."
            show_manual_keyring_instructions
            exit 1
            ;;
    esac

    # Verify installation
    if check_keyring_available; then
        print_success "Keyring dependencies installed successfully"
    else
        print_warning "Packages installed but keyring may need configuration"
        echo
        echo "You may need to:"
        echo "  1. Log out and log back in"
        echo "  2. Or restart your system"
        echo
        echo "Then run 'stronghold install' again."
        exit 1
    fi
}

# Download and install
download_and_install() {
    print_info "Downloading Stronghold..."

    # Determine download URL
    TARBALL_NAME="stronghold-$PLATFORM-$ARCH.tar.gz"
    if [ "$VERSION" = "latest" ]; then
        DOWNLOAD_URL="$REPO_URL/releases/latest/download/$TARBALL_NAME"
        CHECKSUMS_URL="$REPO_URL/releases/latest/download/checksums.txt"
    else
        DOWNLOAD_URL="$REPO_URL/releases/download/$VERSION/$TARBALL_NAME"
        CHECKSUMS_URL="$REPO_URL/releases/download/$VERSION/checksums.txt"
    fi

    # Create temp directory
    TMP_DIR=$(mktemp -d)
    trap "rm -rf $TMP_DIR" EXIT

    # Download tarball
    if ! curl -fsSL "$DOWNLOAD_URL" -o "$TMP_DIR/$TARBALL_NAME"; then
        print_error "Failed to download Stronghold"
        print_info "If you're building from source, run: go build ./cmd/cli && go build ./cmd/proxy"
        exit 1
    fi

    # Download and verify checksums
    if curl -fsSL "$CHECKSUMS_URL" -o "$TMP_DIR/checksums.txt" 2>/dev/null; then
        print_info "Verifying download integrity..."

        # Use sha256sum on Linux, shasum on macOS
        if command -v sha256sum &> /dev/null; then
            SHA_CMD="sha256sum"
        elif command -v shasum &> /dev/null; then
            SHA_CMD="shasum -a 256"
        else
            print_warning "No sha256sum or shasum found, skipping checksum verification"
            SHA_CMD=""
        fi

        if [ -n "$SHA_CMD" ]; then
            # Extract the expected checksum for our tarball
            EXPECTED=$(grep "$TARBALL_NAME" "$TMP_DIR/checksums.txt" | awk '{print $1}')
            if [ -z "$EXPECTED" ]; then
                print_error "Tarball not found in checksums.txt"
                exit 1
            fi

            ACTUAL=$($SHA_CMD "$TMP_DIR/$TARBALL_NAME" | awk '{print $1}')
            if [ "$EXPECTED" != "$ACTUAL" ]; then
                print_error "Checksum verification failed!"
                print_error "Expected: $EXPECTED"
                print_error "Actual:   $ACTUAL"
                exit 1
            fi
            print_success "Checksum verified"
        fi
    else
        print_warning "Checksums not available for this release, skipping verification"
    fi

    # Extract
    tar -xzf "$TMP_DIR/$TARBALL_NAME" -C "$TMP_DIR"

    # Install binaries
    print_info "Installing binaries..."

    # Check if we need sudo
    if [ -w "$INSTALL_DIR" ]; then
        SUDO=""
    else
        print_warning "Need sudo access to install to $INSTALL_DIR"
        SUDO="sudo"
    fi

    # Install CLI
    $SUDO install -m 755 "$TMP_DIR/stronghold" "$INSTALL_DIR/stronghold"
    print_success "Installed stronghold CLI to $INSTALL_DIR/stronghold"

    # Install proxy
    $SUDO install -m 755 "$TMP_DIR/stronghold-proxy" "$INSTALL_DIR/stronghold-proxy"
    print_success "Installed stronghold-proxy to $INSTALL_DIR/stronghold-proxy"
}

# Run post-install setup
post_install() {
    print_info "Running post-install setup..."

    # Add to PATH if needed
    if [[ ":$PATH:" != *":$INSTALL_DIR:"* ]]; then
        print_warning "$INSTALL_DIR is not in your PATH"
        print_info "Add the following to your shell profile:"
        echo "  export PATH=\"$INSTALL_DIR:\$PATH\""
    fi

    print_success "Installation complete!"
    echo
    echo "To get started, run:"
    echo "  stronghold install"
    echo
    echo "For help:"
    echo "  stronghold --help"
}

# Build from source if in a git repo
build_from_source() {
    print_info "Building from source..."

    if ! command -v go &> /dev/null; then
        print_error "Go is required to build from source"
        exit 1
    fi

    # Check if we're in the stronghold repo
    if [ ! -f "go.mod" ] || ! grep -q "module stronghold" go.mod 2>/dev/null; then
        print_error "Not in the stronghold repository"
        exit 1
    fi

    # Build CLI
    go build -o "$TMP_DIR/stronghold" ./cmd/cli
    print_success "Built stronghold CLI"

    # Build proxy
    go build -o "$TMP_DIR/stronghold-proxy" ./cmd/proxy
    print_success "Built stronghold-proxy"

    # Install
    if [ -w "$INSTALL_DIR" ]; then
        SUDO=""
    else
        SUDO="sudo"
    fi

    $SUDO install -m 755 "$TMP_DIR/stronghold" "$INSTALL_DIR/stronghold"
    $SUDO install -m 755 "$TMP_DIR/stronghold-proxy" "$INSTALL_DIR/stronghold-proxy"
}

# Main installation flow
main() {
    echo
    echo "╔══════════════════════════════════════════╗"
    echo "║       Stronghold Installer               ║"
    echo "║   AI Security for LLM Agents             ║"
    echo "╚══════════════════════════════════════════╝"
    echo

    detect_platform
    check_dependencies
    install_keyring_deps

    # Check if we should build from source
    if [ "${BUILD_FROM_SOURCE:-false}" = "true" ] || [ -f "go.mod" ] && grep -q "module stronghold" go.mod 2>/dev/null; then
        TMP_DIR=$(mktemp -d)
        trap "rm -rf $TMP_DIR" EXIT
        build_from_source
    else
        download_and_install
    fi

    post_install
}

# Run main function
main "$@"
