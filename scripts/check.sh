#!/usr/bin/env bash
set -euo pipefail
root_dir="$(cd "$(dirname "$0")/.." && pwd)"
test_dir="$(mktemp -d "${TMPDIR:-/tmp}/passpine-neo-check.XXXXXX")"
trap 'rm -f "$test_dir/passpine-host-test"; rmdir "$test_dir"' EXIT
cd "$root_dir/client"
go build -o "$test_dir/passpine-host-test" ./cmd/passpine-host
PASSPINE_NEO_TEST_HOST="$test_dir/passpine-host-test" go test -race ./...
go vet ./...
cd "$root_dir/browser"
npm run typecheck
npm test
