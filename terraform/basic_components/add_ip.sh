#!/bin/bash
set -e

PREFIX_LIST_ID=$1
MY_IP=$2

# Fetch the current prefix list version and entries
CURRENT_VERSION=$(aws ec2 describe-managed-prefix-lists --prefix-list-id "$PREFIX_LIST_ID" --query 'PrefixLists[0].Version' --output text)
EXISTING_IPS=$(aws ec2 describe-managed-prefix-lists --prefix-list-id "$PREFIX_LIST_ID" --query 'PrefixLists[0].Entries[*].Cidr' --output text)

# Add IP if not present
if echo "$EXISTING_IPS" | grep -q "$MY_IP/32"; then
  echo "$MY_IP/32 is already present in the prefix list."
else
  aws ec2 modify-managed-prefix-list \
    --prefix-list-id "$PREFIX_LIST_ID" \
    --current-version "$CURRENT_VERSION" \
    --add-entries Cidr="$MY_IP/32",Description="Added by Terraform"
  echo "Successfully added $MY_IP/32 to the prefix list."
fi
