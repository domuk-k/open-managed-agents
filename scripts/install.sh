#!/bin/bash
set -euo pipefail

# Detect OS and arch
OS=$(uname -s | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m)
case $ARCH in
  x86_64) ARCH="amd64" ;;
  aarch64|arm64) ARCH="arm64" ;;
esac

VERSION=${OMA_VERSION:-latest}
if [ "$VERSION" = "latest" ]; then
  VERSION=$(curl -fsSL https://api.github.com/repos/domuk-k/open-managed-agents/releases/latest | grep tag_name | cut -d'"' -f4)
fi

URL="https://github.com/domuk-k/open-managed-agents/releases/download/${VERSION}/oma_${VERSION#v}_${OS}_${ARCH}.tar.gz"

echo "Downloading OMA ${VERSION} for ${OS}/${ARCH}..."
curl -fsSL "$URL" | tar xz -C /tmp
sudo mv /tmp/oma /usr/local/bin/oma
echo "OMA installed successfully! Run 'oma --help' to get started."
