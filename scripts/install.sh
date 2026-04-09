#!/bin/bash
set -euo pipefail

REPO="domuk-k/open-managed-agents"

# ---------------------------------------------------------------------------
# Help
# ---------------------------------------------------------------------------

usage() {
  cat <<EOF
Usage: install.sh [OPTIONS]

Install the OMA CLI binary.

Options:
  --version VERSION   Install a specific version (default: latest)
  --no-verify         Skip checksum verification
  --verify            (no-op, verification is on by default)
  --help              Show this help message

Environment variables:
  OMA_VERSION         Same as --version
EOF
  exit 0
}

# ---------------------------------------------------------------------------
# Parse flags
# ---------------------------------------------------------------------------

VERIFY=true
VERSION="${OMA_VERSION:-latest}"

while [ $# -gt 0 ]; do
  case "$1" in
    --help|-h) usage ;;
    --no-verify) VERIFY=false; shift ;;
    --verify)  shift ;; # no-op, verification is on by default
    --version) VERSION="$2"; shift 2 ;;
    *) echo "Unknown option: $1"; usage ;;
  esac
done

# ---------------------------------------------------------------------------
# Detect OS and arch
# ---------------------------------------------------------------------------

OS=$(uname -s | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m)
case $ARCH in
  x86_64)         ARCH="amd64" ;;
  aarch64|arm64)  ARCH="arm64" ;;
esac

if [ "$OS" = "windows" ] || [[ "$OS" == mingw* ]]; then
  echo "Error: Windows is not supported." >&2
  exit 1
fi

# ---------------------------------------------------------------------------
# Resolve version
# ---------------------------------------------------------------------------

if [ "$VERSION" = "latest" ]; then
  VERSION=$(curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" | grep tag_name | cut -d'"' -f4)
fi

TARBALL="oma_${VERSION#v}_${OS}_${ARCH}.tar.gz"
URL="https://github.com/${REPO}/releases/download/${VERSION}/${TARBALL}"
CHECKSUM_URL="https://github.com/${REPO}/releases/download/${VERSION}/checksums.txt"

echo "Downloading OMA ${VERSION} for ${OS}/${ARCH}..."
curl -fsSL "$URL" -o "/tmp/${TARBALL}"

# ---------------------------------------------------------------------------
# Optional checksum verification
# ---------------------------------------------------------------------------

if [ "$VERIFY" = true ]; then
  echo "Verifying checksum..."
  curl -fsSL "$CHECKSUM_URL" -o /tmp/oma_checksums.txt
  EXPECTED=$(grep "${TARBALL}" /tmp/oma_checksums.txt | awk '{print $1}')
  if command -v sha256sum &>/dev/null; then
    ACTUAL=$(sha256sum "/tmp/${TARBALL}" | awk '{print $1}')
  elif command -v shasum &>/dev/null; then
    ACTUAL=$(shasum -a 256 "/tmp/${TARBALL}" | awk '{print $1}')
  else
    echo "Warning: no sha256sum or shasum found, skipping verification." >&2
    ACTUAL="$EXPECTED"
  fi
  if [ "$EXPECTED" != "$ACTUAL" ]; then
    echo "Error: checksum mismatch (expected ${EXPECTED}, got ${ACTUAL})" >&2
    rm -f "/tmp/${TARBALL}" /tmp/oma_checksums.txt
    exit 1
  fi
  echo "Checksum OK."
  rm -f /tmp/oma_checksums.txt
fi

# ---------------------------------------------------------------------------
# Extract and install
# ---------------------------------------------------------------------------

tar xzf "/tmp/${TARBALL}" -C /tmp
rm -f "/tmp/${TARBALL}"

if command -v sudo &>/dev/null && sudo -n true 2>/dev/null; then
  sudo mv /tmp/oma /usr/local/bin/oma
  echo "OMA installed to /usr/local/bin/oma"
else
  INSTALL_DIR="${HOME}/.local/bin"
  mkdir -p "$INSTALL_DIR"
  mv /tmp/oma "$INSTALL_DIR/oma"
  echo "OMA installed to ${INSTALL_DIR}/oma"
  case ":$PATH:" in
    *":${INSTALL_DIR}:"*) ;;
    *) echo "Note: add ${INSTALL_DIR} to your PATH to use 'oma' directly." ;;
  esac
fi

echo "Run 'oma --help' to get started."
