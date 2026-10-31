#!/usr/bin/env bash
set -euo pipefail
root_dir="$(cd "$(dirname "$0")/.." && pwd -P)"
IFS=$'\t' read -r channel version base_version output_dir app_name bundle_id package_name client_flags host_flags build_tags < <(node "$root_dir/scripts/build-channel.mjs" shell)
[[ "$channel" == "dev" ]] || { echo "Local installation accepts development builds only. Install formal releases from GitHub Releases." >&2; exit 1; }

case "$(uname -s)" in
  Darwin)
    install_dir="${1:-$HOME/Applications}"
    source_bundle="$output_dir/$app_name.app"
    [[ -d "$source_bundle" ]] || { echo "Missing client build. Run make client:build first." >&2; exit 1; }
    [[ "$(/usr/libexec/PlistBuddy -c 'Print CFBundleIdentifier' "$source_bundle/Contents/Info.plist")" == "$bundle_id" ]] || { echo "Refusing a client bundle with a different application identity." >&2; exit 1; }
    resolved_signing="$(node "$root_dir/scripts/macos-dev-signing.mjs" resolve)"
    signing_fingerprint="$(node --input-type=module -e 'console.log(JSON.parse(process.argv[1]).certificateSHA256)' "$resolved_signing")"
    node "$root_dir/scripts/macos-signing.mjs" verify-dev "$source_bundle" "$signing_fingerprint"
    [[ "$("$source_bundle/Contents/MacOS/passpine-host" --build-channel)" == "dev" ]] || { echo "Refusing to install a non-development native host." >&2; exit 1; }
    ;;
  Linux)
    install_dir="${1:-$HOME/.local/bin}"
    ;;
  *)
    echo "Client installation is supported on macOS and Linux." >&2
    exit 1
    ;;
esac

node "$root_dir/scripts/client-installation.mjs" preflight "$install_dir"

mkdir -p "$install_dir"
install_dir="$(cd "$install_dir" && pwd -P)"
case "$install_dir/" in
  "$root_dir/"*) echo "Client installation directory must be outside the repository." >&2; exit 1 ;;
esac

if [[ "$(uname -s)" == "Darwin" ]]; then
  destination="$install_dir/$app_name.app"
  if [[ -L "$destination" || ( -e "$destination" && ! -d "$destination" ) ]]; then
    echo "Client destination must be a real directory, not a symlink or file: $destination" >&2
    exit 1
  fi
  if [[ -e "$destination" ]]; then
    [[ "$(/usr/libexec/PlistBuddy -c 'Print CFBundleIdentifier' "$destination/Contents/Info.plist")" == "$bundle_id" ]] || { echo "Refusing to replace a bundle with a different application identity." >&2; exit 1; }
    [[ "$("$destination/Contents/MacOS/passpine-host" --build-channel)" == "dev" ]] || { echo "Refusing to replace a non-development installation." >&2; exit 1; }
  fi
  staging="$(mktemp -d "$install_dir/.passpine-dev.XXXXXX")"
  backup="$staging/previous.app"
  cleanup() {
    if [[ -e "$backup" ]]; then
      echo "Previous client preserved at: $backup" >&2
    else
      rm -rf "$staging"
    fi
  }
  trap cleanup EXIT
  ditto "$source_bundle" "$staging/$app_name.app"
  node "$root_dir/scripts/macos-signing.mjs" verify-dev "$staging/$app_name.app" "$signing_fingerprint"
  if [[ -e "$destination" ]]; then
    mv "$destination" "$backup"
  fi
  if ! mv "$staging/$app_name.app" "$destination"; then
    if [[ -e "$backup" ]]; then
      mv "$backup" "$destination"
    fi
    exit 1
  fi
  rm -rf "$backup"
  host="$destination/Contents/MacOS/passpine-host"
else
  for binary in passpine-dev passpine-dev-host; do
    if [[ -L "$install_dir/$binary" || ( -e "$install_dir/$binary" && ! -f "$install_dir/$binary" ) ]]; then
      echo "Client destination must be a regular file: $install_dir/$binary" >&2
      exit 1
    fi
    if [[ -e "$install_dir/$binary" ]]; then
      [[ "$("$install_dir/$binary" --build-channel)" == "dev" ]] || { echo "Refusing to replace a non-development installation: $install_dir/$binary" >&2; exit 1; }
    fi
  done
  for binary in passpine passpine-host; do
    [[ -f "$output_dir/$binary" ]] || { echo "Missing client build. Run make client:build first." >&2; exit 1; }
    [[ "$("$output_dir/$binary" --build-channel)" == "dev" ]] || { echo "Refusing to install a non-development executable." >&2; exit 1; }
  done
  install -m 755 "$output_dir/passpine" "$install_dir/passpine-dev"
  install -m 755 "$output_dir/passpine-host" "$install_dir/passpine-dev-host"
  destination="$install_dir/passpine-dev"
  host="$install_dir/passpine-dev-host"

  data_dir="${XDG_DATA_HOME:-$HOME/.local/share}"
  [[ "$data_dir" == /* ]] || { echo "XDG_DATA_HOME must be an absolute path." >&2; exit 1; }
  mkdir -p "$data_dir/applications" "$data_dir/icons/hicolor/256x256/apps"
  install -m 644 "$root_dir/client/assets/dev/monochrome/app-icon-dev.png" "$data_dir/icons/hicolor/256x256/apps/$bundle_id.png"
  # Desktop Entry values and Exec arguments have separate escaping rules.
  node --input-type=module - "$destination" "$data_dir/applications/$bundle_id.desktop" "$app_name" "$bundle_id" <<'NODE'
import fs from 'node:fs'
const [executable, output, name, icon] = process.argv.slice(2)
const argument = executable.replace(/[\\"`$]/g, value => '\\' + value).replace(/%/g, '%%')
const exec = `"${argument}"`.replace(/\\/g, '\\\\').replace(/\n/g, '\\n').replace(/\r/g, '\\r').replace(/\t/g, '\\t')
fs.writeFileSync(output, `[Desktop Entry]\nType=Application\nName=${name}\nExec=${exec}\nIcon=${icon}\nTerminal=false\nCategories=Utility;Security;\n`, {mode: 0o644})
NODE
  if command -v update-desktop-database >/dev/null 2>&1; then
    update-desktop-database "$data_dir/applications"
  fi
fi

"$host" --register
node "$root_dir/scripts/client-installation.mjs" record "$install_dir"
echo "Installed client: $destination"

# A fictional vault gives the development client and CLI something to test
# against. Seeding is idempotent and never blocks the installation.
if [[ "${PASSPINE_SKIP_TEST_VAULT:-}" != "1" ]]; then
  client="$destination"
  [[ -d "$destination" ]] && client="$destination/Contents/MacOS/passpine"
  "$client" --seed-test-vault || echo "Test vault was not created; rerun: \"$client\" --seed-test-vault" >&2
fi
