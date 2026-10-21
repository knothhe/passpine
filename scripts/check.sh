#!/usr/bin/env bash
set -euo pipefail
root_dir="$(cd "$(dirname "$0")/.." && pwd)"
test_dir="$(mktemp -d "${TMPDIR:-/tmp}/passpine-check.XXXXXX")"
trap 'rm -rf "$test_dir"' EXIT
cd "$root_dir"
node scripts/version.mjs check
node scripts/dependencies.mjs --check
node --test scripts/version.test.mjs scripts/release.test.mjs scripts/release-artifacts.test.mjs scripts/dependencies.test.mjs
cd "$root_dir/scripts/windows-resources"
go test ./...
cd "$root_dir/client"
if [[ "$(uname -s)" == "Darwin" ]]; then
  xcrun swiftc -O -parse-as-library -swift-version 5 \
    -module-cache-path "$test_dir/swift-module-cache" \
    native/macos/DeviceUnlockEnvelope.swift native/macos/DeviceUnlockTests.swift \
    -o "$test_dir/device-unlock-test"
  "$test_dir/device-unlock-test"
fi
go build -o "$test_dir/passpine-host-test" ./cmd/passpine-host
PASSPINE_TEST_HOST="$test_dir/passpine-host-test" go test -race ./...
go vet ./...
cd "$root_dir/browser"
pnpm run typecheck
pnpm test
