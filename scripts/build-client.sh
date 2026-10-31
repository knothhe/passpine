#!/usr/bin/env bash
set -euo pipefail
root_dir="$(cd "$(dirname "$0")/.." && pwd)"
if [[ "$(uname -s)" == "Darwin" ]]; then
  if [[ "${CGO_ENABLED-1}" != "1" ]]; then
    echo "macOS Passpine builds require CGO_ENABLED=1 for R2 Keychain storage; unset CGO_ENABLED or set it to 1, and install Xcode Command Line Tools." >&2
    exit 1
  fi
  # The SDK's macOS Keychain backend is selected by darwin && cgo. Override an
  # implicit per-user go env setting so local and release packages agree.
  export CGO_ENABLED=1
fi
IFS=$'\t' read -r channel version base_version output_dir app_name bundle_id package_name client_flags host_flags build_tags < <(node "$root_dir/scripts/build-channel.mjs" shell)
if [[ "$(uname -s)" == "Darwin" ]]; then
  if [[ -n "${PASSPINE_MACOS_SIGNING_KEYCHAIN:-}" ]]; then
    # Resolve before entering client/ so discovery and codesign use the same
    # explicit keychain even when the caller supplies a relative path.
    PASSPINE_MACOS_SIGNING_KEYCHAIN="$(node --input-type=module -e 'import path from "node:path"; console.log(path.resolve(process.argv[1]))' "$PASSPINE_MACOS_SIGNING_KEYCHAIN")"
    export PASSPINE_MACOS_SIGNING_KEYCHAIN
  fi
  if [[ "$channel" == "dev" ]]; then
    resolved_signing="$(node "$root_dir/scripts/macos-dev-signing.mjs" resolve)"
    IFS=$'\t' read -r signing_identity signing_fingerprint < <(node --input-type=module - "$resolved_signing" <<'NODE'
const value = JSON.parse(process.argv[2]);
console.log(`${value.identitySHA1}\t${value.certificateSHA256}`);
NODE
)
  else
    signing_identity="${PASSPINE_MACOS_SIGNING_IDENTITY:-}"
    if [[ ! "$signing_identity" =~ ^[[:xdigit:]]{40}$ ]]; then
      echo "Release macOS builds must use an exact pinned signing identity: run node scripts/macos-signing.mjs build." >&2
      exit 1
    fi
  fi
  signing=(--sign "$signing_identity" --timestamp=none)
  [[ -z "${PASSPINE_MACOS_SIGNING_KEYCHAIN:-}" ]] || signing+=(--keychain "$PASSPINE_MACOS_SIGNING_KEYCHAIN")
fi
# Identity selection must succeed before build side effects or installation.
node "$root_dir/scripts/dependencies.mjs"
node "$root_dir/scripts/version.mjs" check
node "$root_dir/scripts/dev-icons.mjs"
mkdir -p "$output_dir"
cd "$root_dir/client"
if [[ "$(uname -s)" == "Darwin" ]]; then
  xcrun swiftc -O -parse-as-library -swift-version 5 \
    -target "$(uname -m)-apple-macosx13.0" \
    -module-cache-path "$output_dir/swift-module-cache" \
    native/macos/DeviceUnlockEnvelope.swift native/macos/DeviceUnlockMain.swift \
    -o "$output_dir/passpine-device-unlock"
fi
go build -trimpath -tags="$build_tags" -ldflags="-s -w $client_flags" -o "$output_dir/passpine" ./cmd/passpine
go build -trimpath -ldflags="-s -w $host_flags" -o "$output_dir/passpine-host" ./cmd/passpine-host
if [[ "$(uname -s)" == "Darwin" ]]; then
  bundle="$output_dir/$app_name.app"
  mkdir -p "$bundle/Contents/MacOS" "$bundle/Contents/Resources"
  cp "$output_dir/passpine" "$bundle/Contents/MacOS/passpine"
  cp "$output_dir/passpine-host" "$bundle/Contents/MacOS/passpine-host"
  cp "$output_dir/passpine-device-unlock" "$bundle/Contents/MacOS/passpine-device-unlock"
  icon="$root_dir/client/assets/Passpine-monochrome.icns"
  [[ "$channel" != "dev" ]] || icon="$root_dir/client/assets/dev/monochrome/app-icon-dev.icns"
  cp "$icon" "$bundle/Contents/Resources/Passpine.icns"
  cp "$icon" "$bundle/Contents/Resources/Passpine-monochrome.icns"
  cat > "$bundle/Contents/Info.plist" <<PLIST
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
<key>CFBundleName</key><string>$app_name</string>
<key>CFBundleDisplayName</key><string>$app_name</string>
<key>CFBundleIdentifier</key><string>$bundle_id</string>
<key>CFBundleExecutable</key><string>passpine</string>
<key>CFBundleIconFile</key><string>Passpine.icns</string>
<key>CFBundlePackageType</key><string>APPL</string>
<key>CFBundleShortVersionString</key><string>$base_version</string>
<key>CFBundleVersion</key><string>$base_version</string>
<key>PasspineBuildChannel</key><string>$channel</string>
<key>LSMinimumSystemVersion</key><string>13.0</string>
<key>NSHighResolutionCapable</key><true/>
<key>NSPrincipalClass</key><string>NSApplication</string>
</dict></plist>
PLIST
  codesign --force "${signing[@]}" --identifier "$bundle_id.host" "$bundle/Contents/MacOS/passpine-host"
  codesign --force "${signing[@]}" --identifier "$bundle_id.device-unlock" "$bundle/Contents/MacOS/passpine-device-unlock"
  codesign --force "${signing[@]}" --identifier "$bundle_id" "$bundle"
  codesign --verify --deep --strict "$bundle"
  if [[ "$channel" == "release" ]]; then
    node "$root_dir/scripts/macos-signing.mjs" verify "$bundle"
  else
    node "$root_dir/scripts/macos-signing.mjs" verify-dev "$bundle" "$signing_fingerprint"
  fi
  ditto -c -k --norsrc --noextattr --noqtn --keepParent "$bundle" "$output_dir/$package_name-$version-macos-$(uname -m).zip"
  echo "Built: $bundle"
elif [[ "$(uname -s)" == "Linux" ]]; then
  "$root_dir/scripts/package-client-linux.sh" "$version" "$(go env GOARCH)" "$output_dir" "$channel"
else
  echo "Built: $output_dir/passpine"
  echo "Built: $output_dir/passpine-host"
fi
node "$root_dir/scripts/build-channel.mjs" write
