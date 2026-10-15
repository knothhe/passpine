#!/usr/bin/env bash
set -euo pipefail
root_dir="$(cd "$(dirname "$0")/.." && pwd)"
mkdir -p "$root_dir/dist"
cd "$root_dir/client"
go build -trimpath -ldflags='-s -w' -o "$root_dir/dist/passpine" ./cmd/passpine
go build -trimpath -ldflags='-s -w' -o "$root_dir/dist/passpine-host" ./cmd/passpine-host
if [[ "$(uname -s)" == "Darwin" ]]; then
  bundle="$root_dir/dist/Passpine Neo.app"
  mkdir -p "$bundle/Contents/MacOS" "$bundle/Contents/Resources"
  cp "$root_dir/dist/passpine" "$bundle/Contents/MacOS/passpine"
  cp "$root_dir/dist/passpine-host" "$bundle/Contents/MacOS/passpine-host"
  cp "$root_dir/client/assets/Passpine.icns" "$bundle/Contents/Resources/Passpine.icns"
  cat > "$bundle/Contents/Info.plist" <<'PLIST'
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
<key>CFBundleName</key><string>Passpine Neo</string>
<key>CFBundleDisplayName</key><string>Passpine Neo</string>
<key>CFBundleIdentifier</key><string>com.knothhe.passpine.neo</string>
<key>CFBundleExecutable</key><string>passpine</string>
<key>CFBundleIconFile</key><string>Passpine.icns</string>
<key>CFBundlePackageType</key><string>APPL</string>
<key>CFBundleShortVersionString</key><string>0.1.0</string>
<key>CFBundleVersion</key><string>1</string>
<key>LSMinimumSystemVersion</key><string>13.0</string>
<key>NSHighResolutionCapable</key><true/>
<key>NSPrincipalClass</key><string>NSApplication</string>
</dict></plist>
PLIST
  codesign --force --deep --sign - "$bundle"
  codesign --verify --deep --strict "$bundle"
  ditto -c -k --sequesterRsrc --keepParent "$bundle" "$root_dir/dist/Passpine-Neo-0.1.0-macos-$(uname -m).zip"
  echo "Built: $bundle"
fi
