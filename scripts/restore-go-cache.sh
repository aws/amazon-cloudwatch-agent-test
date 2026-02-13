#!/bin/bash
# Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
# SPDX-License-Identifier: MIT

# Restores Go build and module caches from S3 to speed up test compilation.
# Usage: ./restore-go-cache.sh <s3_bucket> <cache_key>
# All failures are non-fatal (|| true) to ensure tests still run without cache.

set -euo pipefail

S3_BUCKET="${1:-}"
CACHE_KEY="${2:-}"

if [ -z "$S3_BUCKET" ] || [ -z "$CACHE_KEY" ]; then
  echo "go-cache: skipping restore (no bucket or cache key provided)"
  exit 0
fi

CACHE_PREFIX="s3://${S3_BUCKET}/integration-test/cache/${CACHE_KEY}"
GOCACHE=$(go env GOCACHE 2>/dev/null || echo "$HOME/.cache/go-build")
GOMODCACHE=$(go env GOMODCACHE 2>/dev/null || echo "$HOME/go/pkg/mod")

echo "go-cache: restoring from ${CACHE_PREFIX}"

mkdir -p "$GOCACHE" "$GOMODCACHE"

aws s3 cp "${CACHE_PREFIX}/gocache.tar.gz" /tmp/gocache.tar.gz --quiet 2>/dev/null && \
  tar xzf /tmp/gocache.tar.gz -C "$GOCACHE" 2>/dev/null && \
  rm -f /tmp/gocache.tar.gz && \
  echo "go-cache: build cache restored" || \
  echo "go-cache: build cache not available, will compile from source"

aws s3 cp "${CACHE_PREFIX}/gomodcache.tar.gz" /tmp/gomodcache.tar.gz --quiet 2>/dev/null && \
  tar xzf /tmp/gomodcache.tar.gz -C "$GOMODCACHE" 2>/dev/null && \
  rm -f /tmp/gomodcache.tar.gz && \
  echo "go-cache: module cache restored" || \
  echo "go-cache: module cache not available, will download modules"
