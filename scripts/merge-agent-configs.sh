#!/bin/bash
# Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
# SPDX-License-Identifier: MIT

set -euo pipefail

namespace=""
output=""

while getopts "n:o:" opt; do
  case $opt in
    n) namespace="$OPTARG" ;;
    o) output="$OPTARG" ;;
    *) echo "Usage: $0 [-n namespace] [-o output_file] config1.json config2.json ..." >&2; exit 1 ;;
  esac
done
shift $((OPTIND - 1))

if [ $# -eq 0 ]; then
  echo "Usage: $0 [-n namespace] [-o output_file] config1.json config2.json ..." >&2
  exit 1
fi

if ! command -v jq &>/dev/null; then
  echo "Error: jq is required but not found on PATH" >&2
  exit 1
fi

merged='{}'

for config in "$@"; do
  collected=$(jq '.metrics.metrics_collected' "$config")
  for key in $(echo "$collected" | jq -r 'keys[]'); do
    value=$(echo "$collected" | jq --arg k "$key" '.[$k]')
    if [ "$key" = "procstat" ]; then
      existing=$(echo "$merged" | jq --arg k "$key" '.[$k] // null')
      if [ "$existing" != "null" ]; then
        value=$(jq -n --argjson a "$existing" --argjson b "$value" '$a + $b')
      fi
    fi
    merged=$(echo "$merged" | jq --argjson v "$value" --arg k "$key" '. + {($k): $v}')
  done
done

build_args=(--argjson mc "$merged")
filter='{agent:{metrics_collection_interval:10,run_as_user:"root",debug:true},metrics:{metrics_collected:$mc,force_flush_interval:5}}'

if [ -n "$namespace" ]; then
  build_args+=(--arg ns "$namespace")
  filter='{agent:{metrics_collection_interval:10,run_as_user:"root",debug:true},metrics:{namespace:$ns,metrics_collected:$mc,force_flush_interval:5}}'
fi

result=$(jq -n "${build_args[@]}" "$filter")

if [ -n "$output" ]; then
  echo "$result" > "$output"
else
  echo "$result"
fi
