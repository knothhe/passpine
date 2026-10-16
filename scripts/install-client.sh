#!/usr/bin/env bash
set -euo pipefail
root_dir="$(cd "$(dirname "$0")/.." && pwd -P)"

case "$(uname -s)" in
  Darwin)
    install_dir="${1:-$HOME/Applications}"
    source_bundle="$root_dir/dist/Passpine.app"
    [[ -d "$source_bundle" ]] || { echo "Missing client build. Run make client:build first." >&2; exit 1; }
    ;;
  Linux)
    install_dir="${1:-$HOME/.local/bin}"
    ;;
  *)
    echo "Client installation is supported on macOS and Linux." >&2
    exit 1
    ;;
esac

mkdir -p "$install_dir"
install_dir="$(cd "$install_dir" && pwd -P)"
case "$install_dir/" in
  "$root_dir/"*) echo "Client installation directory must be outside the repository." >&2; exit 1 ;;
esac

if [[ "$(uname -s)" == "Darwin" ]]; then
  destination="$install_dir/Passpine.app"
  if [[ -L "$destination" || ( -e "$destination" && ! -d "$destination" ) ]]; then
    echo "Client destination must be a real directory, not a symlink or file: $destination" >&2
    exit 1
  fi
  staging="$(mktemp -d "$install_dir/.passpine.XXXXXX")"
  backup="$staging/previous.app"
  cleanup() {
    if [[ -e "$backup" ]]; then
      echo "Previous client preserved at: $backup" >&2
    else
      rm -rf "$staging"
    fi
  }
  trap cleanup EXIT
  ditto "$source_bundle" "$staging/Passpine.app"
  codesign --verify --deep --strict "$staging/Passpine.app"
  if [[ -e "$destination" ]]; then
    mv "$destination" "$backup"
  fi
  if ! mv "$staging/Passpine.app" "$destination"; then
    if [[ -e "$backup" ]]; then
      mv "$backup" "$destination"
    fi
    exit 1
  fi
  rm -rf "$backup"
  host="$destination/Contents/MacOS/passpine-host"
else
  for binary in passpine passpine-host; do
    if [[ -L "$install_dir/$binary" || ( -e "$install_dir/$binary" && ! -f "$install_dir/$binary" ) ]]; then
      echo "Client destination must be a regular file: $install_dir/$binary" >&2
      exit 1
    fi
    [[ -f "$root_dir/dist/$binary" ]] || { echo "Missing client build. Run make client:build first." >&2; exit 1; }
  done
  install -m 755 "$root_dir/dist/passpine" "$install_dir/passpine"
  install -m 755 "$root_dir/dist/passpine-host" "$install_dir/passpine-host"
  destination="$install_dir/passpine"
  host="$install_dir/passpine-host"
fi

"$host" --register
echo "Installed client: $destination"
