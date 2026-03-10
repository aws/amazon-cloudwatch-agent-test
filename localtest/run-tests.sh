#!/bin/bash
set -e

cd /workspace

# Wait for systemd to be ready
timeout=30
elapsed=0
until systemctl is-system-running --wait 2>/dev/null; do
  elapsed=$((elapsed + 1))
  if [ "$elapsed" -ge "$timeout" ]; then
    echo "ERROR: systemd not ready after ${timeout}s"
    exit 1
  fi
  sleep 1
done

go test -v -timeout 30m ./test/metric_value_benchmark/... \
  -args -computeType=EC2 \
  -plugins="${PLUGINS:-CPU,Mem}" \
  -agentStartCommand="sudo /opt/aws/amazon-cloudwatch-agent/bin/amazon-cloudwatch-agent-ctl -a fetch-config -m onPrem -s -c file:/opt/aws/amazon-cloudwatch-agent/bin/config.json"
