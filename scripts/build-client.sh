#!/usr/bin/env bash
set -euo pipefail
root_dir="$(cd "$(dirname "$0")/.." && pwd)"
IFS=$'\t' read -r channel version base_version output_dir app_name bundle_id package_name client_flags host_flags < <(node "$root_dir/scripts/build-channel.mjs" shell)
node "$root_dir/scripts/dependencies.mjs"
node "$root_dir/scripts/version.mjs" check
mkdir -p "$output_dir"
cd "$root_dir/client"
if [[ "$(uname -s)" == "Darwin" ]]; then
  xcrun swiftc -O -parse-as-library -swift-version 5 \
    -target "$(uname -m)-apple-macosx13.0" \
    -module-cache-path "$output_dir/swift-module-cache" \
    native/macos/DeviceUnlockEnvelope.swift native/macos/DeviceUnlockMain.swift \
    -o "$output_dir/passpine-device-unlock"
fi
go build -trimpath -ldflags="-s -w $client_flags" -o "$output_dir/passpine" ./cmd/passpine
go build -trimpath -ldflags="-s -w $host_flags" -o "$output_dir/passpine-host" ./cmd/passpine-host
if [[ "$(uname -s)" == "Darwin" ]]; then
  bundle="$output_dir/$app_name.app"
  mkdir -p "$bundle/Contents/MacOS" "$bundle/Contents/Resources"
  cp "$output_dir/passpine" "$bundle/Contents/MacOS/passpine"
  cp "$output_dir/passpine-host" "$bundle/Contents/MacOS/passpine-host"
  cp "$output_dir/passpine-device-unlock" "$bundle/Contents/MacOS/passpine-device-unlock"
  cp "$root_dir/client/assets/Passpine-monochrome.icns" "$bundle/Contents/Resources/Passpine.icns"
  cp "$root_dir/client/assets/Passpine-monochrome.icns" "$bundle/Contents/Resources/Passpine-monochrome.icns"
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
  codesign --force --deep --sign - "$bundle"
  codesign --verify --deep --strict "$bundle"
  ditto -c -k --norsrc --noextattr --noqtn --keepParent "$bundle" "$output_dir/$package_name-$version-macos-$(uname -m).zip"
  echo "Built: $bundle"
elif [[ "$(uname -s)" == "Linux" ]]; then
  "$root_dir/scripts/package-client-linux.sh" "$version" "$(go env GOARCH)" "$output_dir" "$channel"
else
  echo "Built: $output_dir/passpine"
  echo "Built: $output_dir/passpine-host"
fi
node "$root_dir/scripts/build-channel.mjs" write
