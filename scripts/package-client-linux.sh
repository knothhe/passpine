#!/usr/bin/env bash
set -euo pipefail
root_dir="$(cd "$(dirname "$0")/.." && pwd -P)"

if [[ "$#" -lt 2 || "$#" -gt 3 ]]; then
  echo "Usage: package-client-linux.sh VERSION ARCH [DIST_DIR]" >&2
  exit 1
fi
version="$1"
arch="$2"
dist_dir="${3:-$root_dir/dist}"
[[ "$version" =~ ^[0-9]+\.[0-9]+\.[0-9]+([+-][0-9A-Za-z.+-]+)?$ ]] || { echo "Invalid release version: $version" >&2; exit 1; }
case "$arch" in
  amd64|arm64) ;;
  *) echo "Unsupported Linux release architecture: $arch" >&2; exit 1 ;;
esac
for binary in passpine passpine-host; do
  [[ -f "$dist_dir/$binary" && -x "$dist_dir/$binary" && ! -L "$dist_dir/$binary" ]] || { echo "Missing executable client build: $dist_dir/$binary" >&2; exit 1; }
done
dist_dir="$(cd "$dist_dir" && pwd -P)"
staging="$(mktemp -d "$dist_dir/.passpine-linux.XXXXXX")"
trap 'rm -rf "$staging"' EXIT
install -m 755 "$dist_dir/passpine" "$staging/passpine"
install -m 755 "$dist_dir/passpine-host" "$staging/passpine-host"
install -m 755 "$root_dir/scripts/install-linux-release.sh" "$staging/install.sh"
install -m 644 "$root_dir/client/assets/app-icon-monochrome.png" "$staging/app-icon-monochrome.png"
printf '%s\n' "$version" > "$staging/version.txt"
archive="$dist_dir/Passpine-$version-linux-$arch.tar.gz"
tar -czf "$staging/archive.tar.gz" -C "$staging" passpine passpine-host version.txt app-icon-monochrome.png install.sh
mv "$staging/archive.tar.gz" "$archive"
echo "Built: $archive"
