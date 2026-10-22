#!/usr/bin/env bash
set -euo pipefail
source_dir="$(cd "$(dirname "$0")" && pwd -P)"
[[ "$(uname -s)" == "Linux" ]] || { echo "This release installs on Linux only." >&2; exit 1; }
[[ "$#" -le 1 ]] || { echo "Usage: ./install.sh [INSTALL_DIR]" >&2; exit 1; }
channel="release"
if [[ -f "$source_dir/channel.txt" ]]; then
  channel="$(cat "$source_dir/channel.txt")"
fi
case "$channel" in
  dev) binary_name="passpine-dev"; host_name="passpine-dev-host"; app_name="Passpine Dev"; app_id="com.knothhe.passpine.dev" ;;
  release) binary_name="passpine"; host_name="passpine-host"; app_name="Passpine"; app_id="com.knothhe.passpine" ;;
  *) echo "Invalid package build channel." >&2; exit 1 ;;
esac

install_dir="${1:-$HOME/.local/bin}"
data_dir="${XDG_DATA_HOME:-$HOME/.local/share}"
[[ "$data_dir" == /* ]] || { echo "XDG_DATA_HOME must be an absolute path." >&2; exit 1; }
for binary in passpine passpine-host; do
  [[ -f "$source_dir/$binary" && -x "$source_dir/$binary" ]] || { echo "Missing executable from release: $binary" >&2; exit 1; }
done
[[ -f "$source_dir/app-icon-monochrome.png" ]] || { echo "Missing release icon." >&2; exit 1; }
mkdir -p "$install_dir"
install_dir="$(cd "$install_dir" && pwd -P)"
case "$install_dir/" in
  "$source_dir/"*) echo "Install the client outside the extracted release directory." >&2; exit 1 ;;
esac
for binary in "$binary_name" "$host_name"; do
  if [[ -L "$install_dir/$binary" || ( -e "$install_dir/$binary" && ! -f "$install_dir/$binary" ) ]]; then
    echo "Client destination must be a regular file: $install_dir/$binary" >&2
    exit 1
  fi
  if [[ "$channel" == "dev" && -e "$install_dir/$binary" ]]; then
    [[ "$("$install_dir/$binary" --build-channel)" == "dev" ]] || { echo "Refusing to replace a non-development executable: $binary" >&2; exit 1; }
  fi
done

install -m 755 "$source_dir/passpine" "$install_dir/$binary_name"
install -m 755 "$source_dir/passpine-host" "$install_dir/$host_name"
mkdir -p "$data_dir/applications" "$data_dir/icons/hicolor/256x256/apps"
install -m 644 "$source_dir/app-icon-monochrome.png" "$data_dir/icons/hicolor/256x256/apps/$app_id.png"

# Escape the Exec argument first, then the Desktop Entry string value.
exec_value="$install_dir/$binary_name"
exec_value="${exec_value//\\/\\\\}"
exec_value="${exec_value//\"/\\\"}"
exec_value="${exec_value//\`/\\\`}"
exec_value="${exec_value//\$/\\\$}"
exec_value="${exec_value//%/%%}"
exec_value="\"$exec_value\""
exec_value="${exec_value//\\/\\\\}"
exec_value="${exec_value//$'\n'/\\n}"
exec_value="${exec_value//$'\r'/\\r}"
exec_value="${exec_value//$'\t'/\\t}"
cat > "$data_dir/applications/$app_id.desktop" <<DESKTOP
[Desktop Entry]
Type=Application
Name=$app_name
Exec=$exec_value
Icon=$app_id
Terminal=false
Categories=Utility;Security;
DESKTOP
chmod 644 "$data_dir/applications/$app_id.desktop"
if command -v update-desktop-database >/dev/null 2>&1; then
  update-desktop-database "$data_dir/applications"
fi
"$install_dir/$host_name" --register
echo "Installed client: $install_dir/$binary_name"
