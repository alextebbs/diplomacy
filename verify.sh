#!/usr/bin/env bash
set -euo pipefail

# Verify that the deployed bot matches this source code.
#
# Usage:
#   ./verify.sh <expected-sha256>
#
# Copy the hash from /version in Discord (or curl the /checksum endpoint),
# then run this script from the repo root.

if [ $# -lt 1 ]; then
  echo "Usage: ./verify.sh <expected-sha256>"
  echo ""
  echo "Run /version in Discord or curl /checksum to get the hash, then pass it here."
  exit 1
fi

expected="$1"

docker build --platform linux/amd64 -q -t diplomacy-verify . > /dev/null

actual=$(docker run --rm --platform linux/amd64 --entrypoint sha256sum diplomacy-verify /usr/local/bin/bot | awk '{print $1}')

echo "Expected: $expected"
echo "Got:      $actual"
echo ""

if [ "$expected" = "$actual" ]; then
  echo "PASS - the deployed bot is running this exact source code."
else
  echo "FAIL - the deployed bot is NOT running this source code."
  exit 1
fi
