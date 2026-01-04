#!/bin/sh
# BOP Installer
# Usage: curl -sSfL https://raw.githubusercontent.com/delightfulhammers/bop/main/install.sh | sh
#
# Options (passed after --):
#   -v, --version VERSION   Install specific version (default: latest)
#   -d, --dir DIR           Install directory (default: ~/.local/bin)
#   -h, --help              Show this help message
#
# Examples:
#   curl -sSfL .../install.sh | sh
#   curl -sSfL .../install.sh | sh -s -- --version v0.7.0
#   curl -sSfL .../install.sh | sh -s -- --dir /usr/local/bin

set -e

REPO="delightfulhammers/bop"
INSTALL_DIR="${HOME}/.local/bin"
VERSION=""

# Colors for output (disabled if not a terminal)
if [ -t 1 ]; then
    RED='\033[0;31m'
    GREEN='\033[0;32m'
    YELLOW='\033[0;33m'
    BLUE='\033[0;34m'
    NC='\033[0m' # No Color
else
    RED=''
    GREEN=''
    YELLOW=''
    BLUE=''
    NC=''
fi

info() {
    printf "${BLUE}info${NC}: %s\n" "$1"
}

success() {
    printf "${GREEN}success${NC}: %s\n" "$1"
}

warn() {
    printf "${YELLOW}warn${NC}: %s\n" "$1" >&2
}

error() {
    printf "${RED}error${NC}: %s\n" "$1" >&2
    exit 1
}

usage() {
    cat <<EOF
BOP Installer

Usage: curl -sSfL https://raw.githubusercontent.com/${REPO}/main/install.sh | sh

Options (passed after --):
  -v, --version VERSION   Install specific version (default: latest)
  -d, --dir DIR           Install directory (default: ~/.local/bin)
  -h, --help              Show this help message

Examples:
  # Install latest version
  curl -sSfL https://raw.githubusercontent.com/${REPO}/main/install.sh | sh

  # Install specific version
  curl -sSfL https://raw.githubusercontent.com/${REPO}/main/install.sh | sh -s -- -v v0.7.0

  # Install to custom directory
  curl -sSfL https://raw.githubusercontent.com/${REPO}/main/install.sh | sh -s -- -d /usr/local/bin
EOF
    exit 0
}

# Parse arguments
while [ $# -gt 0 ]; do
    case "$1" in
        -v|--version)
            VERSION="$2"
            shift 2
            ;;
        -d|--dir)
            INSTALL_DIR="$2"
            shift 2
            ;;
        -h|--help)
            usage
            ;;
        *)
            error "Unknown option: $1"
            ;;
    esac
done

# Detect OS
detect_os() {
    OS=$(uname -s | tr '[:upper:]' '[:lower:]')
    case "$OS" in
        darwin)
            echo "darwin"
            ;;
        linux)
            echo "linux"
            ;;
        *)
            error "Unsupported operating system: $OS. Supported: darwin, linux"
            ;;
    esac
}

# Detect architecture
detect_arch() {
    ARCH=$(uname -m)
    case "$ARCH" in
        x86_64|amd64)
            echo "amd64"
            ;;
        aarch64|arm64)
            echo "arm64"
            ;;
        *)
            error "Unsupported architecture: $ARCH. Supported: x86_64/amd64, aarch64/arm64"
            ;;
    esac
}

# Get latest version from GitHub API
get_latest_version() {
    if command -v curl >/dev/null 2>&1; then
        curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" | grep '"tag_name"' | sed -E 's/.*"([^"]+)".*/\1/'
    elif command -v wget >/dev/null 2>&1; then
        wget -qO- "https://api.github.com/repos/${REPO}/releases/latest" | grep '"tag_name"' | sed -E 's/.*"([^"]+)".*/\1/'
    else
        error "Neither curl nor wget found. Please install one of them."
    fi
}

# Download file
download() {
    url="$1"
    output="$2"

    if command -v curl >/dev/null 2>&1; then
        curl -fsSL "$url" -o "$output"
    elif command -v wget >/dev/null 2>&1; then
        wget -q "$url" -O "$output"
    else
        error "Neither curl nor wget found. Please install one of them."
    fi
}

# Verify checksum
verify_checksum() {
    file="$1"
    expected="$2"

    if command -v sha256sum >/dev/null 2>&1; then
        actual=$(sha256sum "$file" | awk '{print $1}')
    elif command -v shasum >/dev/null 2>&1; then
        actual=$(shasum -a 256 "$file" | awk '{print $1}')
    else
        warn "Neither sha256sum nor shasum found. Skipping checksum verification."
        return 0
    fi

    if [ "$actual" != "$expected" ]; then
        error "Checksum verification failed!\nExpected: $expected\nActual:   $actual"
    fi
}

main() {
    OS=$(detect_os)
    ARCH=$(detect_arch)

    info "Detected OS: $OS, Arch: $ARCH"

    # Get version
    if [ -z "$VERSION" ]; then
        info "Fetching latest version..."
        VERSION=$(get_latest_version)
        if [ -z "$VERSION" ]; then
            error "Failed to determine latest version. Try specifying with --version"
        fi
    fi

    info "Installing bop ${VERSION}"

    # Build download URLs
    VERSION_NUM="${VERSION#v}"
    ARCHIVE_NAME="bop_${VERSION_NUM}_${OS}_${ARCH}.tar.gz"
    DOWNLOAD_URL="https://github.com/${REPO}/releases/download/${VERSION}/${ARCHIVE_NAME}"
    CHECKSUM_URL="https://github.com/${REPO}/releases/download/${VERSION}/checksums.txt"

    # Create temp directory
    TMP_DIR=$(mktemp -d)
    # shellcheck disable=SC2064
    trap "rm -rf '$TMP_DIR'" EXIT

    # Download archive and checksums
    info "Downloading ${ARCHIVE_NAME}..."
    download "$DOWNLOAD_URL" "$TMP_DIR/$ARCHIVE_NAME" || error "Failed to download archive. Check version exists: $VERSION"

    info "Downloading checksums..."
    download "$CHECKSUM_URL" "$TMP_DIR/checksums.txt" || error "Failed to download checksums"

    # Verify checksum
    info "Verifying checksum..."
    EXPECTED_CHECKSUM=$(grep "$ARCHIVE_NAME" "$TMP_DIR/checksums.txt" | awk '{print $1}')
    if [ -z "$EXPECTED_CHECKSUM" ]; then
        error "Checksum not found for $ARCHIVE_NAME"
    fi
    verify_checksum "$TMP_DIR/$ARCHIVE_NAME" "$EXPECTED_CHECKSUM"
    success "Checksum verified"

    # Extract archive
    info "Extracting..."
    tar -xzf "$TMP_DIR/$ARCHIVE_NAME" -C "$TMP_DIR"

    # Create install directory
    mkdir -p "$INSTALL_DIR"

    # Install binaries
    info "Installing to $INSTALL_DIR..."

    # Install bop
    if [ -f "$TMP_DIR/bop" ]; then
        mv "$TMP_DIR/bop" "$INSTALL_DIR/bop"
        chmod +x "$INSTALL_DIR/bop"
        success "Installed bop"
    else
        warn "bop binary not found in archive"
    fi

    # Install bop-mcp (if present)
    if [ -f "$TMP_DIR/bop-mcp" ]; then
        mv "$TMP_DIR/bop-mcp" "$INSTALL_DIR/bop-mcp"
        chmod +x "$INSTALL_DIR/bop-mcp"
        success "Installed bop-mcp"
    fi

    # Verify installation
    if [ -x "$INSTALL_DIR/bop" ]; then
        success "bop ${VERSION} installed successfully!"
    fi

    # Check if install dir is in PATH
    case ":$PATH:" in
        *":$INSTALL_DIR:"*)
            ;;
        *)
            echo ""
            warn "$INSTALL_DIR is not in your PATH"
            echo ""
            echo "Add it to your shell profile:"
            echo ""
            echo "  # For bash (~/.bashrc or ~/.bash_profile)"
            echo "  export PATH=\"\$PATH:$INSTALL_DIR\""
            echo ""
            echo "  # For zsh (~/.zshrc)"
            echo "  export PATH=\"\$PATH:$INSTALL_DIR\""
            echo ""
            echo "  # For fish (~/.config/fish/config.fish)"
            echo "  fish_add_path $INSTALL_DIR"
            echo ""
            ;;
    esac

    echo ""
    echo "Run 'bop --help' to get started!"
}

main
