#!/usr/bin/env bash
set -euo pipefail
source_dir="$(cd "$(dirname "$0")" && pwd -P)"
[[ "$(uname -s)" == "Linux" ]] || { echo "This release installs on Linux only." >&2; exit 1; }
[[ "$#" -le 1 ]] || { echo "Usage: ./install.sh [INSTALL_DIR]" >&2; exit 1; }

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
for binary in passpine passpine-host; do
  if [[ -L "$install_dir/$binary" || ( -e "$install_dir/$binary" && ! -f "$install_dir/$binary" ) ]]; then
    echo "Client destination must be a regular file: $install_dir/$binary" >&2
    exit 1
  fi
done

install -m 755 "$source_dir/passpine" "$install_dir/passpine"
install -m 755 "$source_dir/passpine-host" "$install_dir/passpine-host"
mkdir -p "$data_dir/applications" "$data_dir/icons/hicolor/256x256/apps"
install -m 644 "$source_dir/app-icon-monochrome.png" "$data_dir/icons/hicolor/256x256/apps/com.knothhe.passpine.png"

# Escape the Exec argument first, then the Desktop Entry string value.
exec_value="$install_dir/passpine"
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
cat > "$data_dir/applications/com.knothhe.passpine.desktop" <<DESKTOP
[Desktop Entry]
Type=Application
Name=Passpine
Exec=$exec_value
Icon=com.knothhe.passpine
Terminal=false
Categories=Utility;Security;
DESKTOP
chmod 644 "$data_dir/applications/com.knothhe.passpine.desktop"
if command -v update-desktop-database >/dev/null 2>&1; then
  update-desktop-database "$data_dir/applications"
fi
"$install_dir/passpine-host" --register
echo "Installed client: $install_dir/passpine"
