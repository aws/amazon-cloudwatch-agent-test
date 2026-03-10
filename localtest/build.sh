#!/bin/bash
set -euo pipefail

SCRIPT_DIR=$(cd "$(dirname "$0")" && pwd)
AGENT_REPO=${AGENT_REPO:-${SCRIPT_DIR}/../../amazon-cloudwatch-agent}

case "$(uname -m)" in
  x86_64)  ARCH=amd64 ;;
  aarch64) ARCH=arm64 ;;
  *) echo "Unsupported architecture: $(uname -m)"; exit 1 ;;
esac

cd "$AGENT_REPO"
make amazon-cloudwatch-agent-linux package-rpm

mkdir -p "${SCRIPT_DIR}/.build"
cp "build/bin/linux/${ARCH}/amazon-cloudwatch-agent.rpm" "${SCRIPT_DIR}/.build/"
echo "RPM copied to ${SCRIPT_DIR}/.build/amazon-cloudwatch-agent.rpm"

cd "$SCRIPT_DIR"
docker-compose build
docker-compose up -d

echo "Container started. Attach with: docker-compose exec cwagent-test bash"
