#!/bin/sh
# One-step installer for the official Linux release:
#   curl -fsSL https://raw.githubusercontent.com/knothhe/passpine/main/scripts/install-linux.sh | sh
# Installs GTK 3 and Git with the distribution's package manager, verifies the
# release's signed update feed and package, then runs the package's own
# install.sh into ~/.local/bin where the in-app updater can replace it.
# Pass options after `sh -s --`: --version X.Y.Z, --skip-deps, --uninstall.
set -eu

# Everything runs from main, so a truncated download executes nothing.
fail() { printf '%s\n' "passpine: $*" >&2; exit 1; }
say() { printf '%s\n' "passpine: $*"; }

usage() {
 cat <<'EOF'
Usage: install-linux.sh [--version X.Y.Z] [--skip-deps] [--uninstall]

  --version X.Y.Z  install this release instead of the latest one
  --skip-deps      do not install GTK 3, Git and OpenSSL with the package manager
  --uninstall      unregister the browser connection and remove the programs,
                   desktop entry and icon; keep vaults, keys and settings

Environment: PASSPINE_BIN_DIR (default ~/.local/bin), XDG_DATA_HOME.
EOF
}

app_id=com.knothhe.passpine
official_releases_url=https://github.com/knothhe/passpine/releases
# Same pinned Ed25519 key as config/update-signing.json.
official_public_key=0V47gc+qVVlC4On/zR4jROkji3RPnzxC0YavPDzYoWQ=
releases_url=${PASSPINE_RELEASES_URL:-$official_releases_url}
public_key=$official_public_key
# A test key is honoured only for a non-official release source.
if [ "$releases_url" != "$official_releases_url" ] && [ -n "${PASSPINE_INSTALL_PUBLIC_KEY:-}" ]; then
 public_key=$PASSPINE_INSTALL_PUBLIC_KEY
fi

fetch() {
 if command -v curl >/dev/null 2>&1; then
  curl -fsSL --retry 3 --proto-redir =https -o "$2" "$1"
 elif command -v wget >/dev/null 2>&1; then
  wget -q -O "$2" "$1"
 else
  fail 'curl or wget is required'
 fi
}

sha256() {
 if command -v sha256sum >/dev/null 2>&1; then sha256sum "$1"; else shasum -a 256 "$1"; fi | cut -d ' ' -f 1
}

# Prints "manager package..." for the missing runtime packages, or nothing.
missing_packages() {
 os_release=${PASSPINE_OS_RELEASE:-/etc/os-release}
 [ -r "$os_release" ] || os_release=/usr/lib/os-release
 [ -r "$os_release" ] || return 2
 ids=$(. "$os_release"; printf '%s %s' "${ID:-}" "${ID_LIKE:-}")
 case " $ids " in
  *' arch '*) manager=pacman; gtk=gtk3 ;;
  *' debian '*|*' ubuntu '*) manager=apt-get; gtk=libgtk-3-0 ;;
  *' suse '*|*' opensuse '*) manager=zypper; gtk=libgtk-3-0 ;;
  *' fedora '*|*' rhel '*|*' centos '*) manager=dnf; gtk=gtk3 ;;
  *) return 2 ;;
 esac
 command -v "$manager" >/dev/null 2>&1 || return 2
 packages=''
 for package in "$gtk" git openssl; do
  case "$package" in git|openssl) if command -v "$package" >/dev/null 2>&1; then continue; fi ;; esac
  case "$manager" in
   pacman) pacman -Q "$package" >/dev/null 2>&1 && continue ;;
   # Ubuntu 24.04 and Debian 13 ship GTK 3 as libgtk-3-0t64.
   apt-get) for name in "$package" "${package}t64"; do
     dpkg-query -W -f='${Status}' "$name" 2>/dev/null | grep -q 'ok installed' && continue 2
    done ;;
   *) rpm -q "$package" >/dev/null 2>&1 && continue ;;
  esac
  packages="$packages $package"
 done
 [ -z "$packages" ] || printf '%s%s\n' "$manager" "$packages"
}

install_dependencies() {
 status=0
 missing=$(missing_packages) || status=$?
 if [ "$status" -ne 0 ]; then
  say 'unrecognised distribution; install GTK 3, Git and OpenSSL yourself, then rerun with --skip-deps'
  fail 'GTK 3 package names: Arch/Fedora gtk3, Debian/Ubuntu/openSUSE libgtk-3-0'
 fi
 [ -n "$missing" ] || return 0
 set -- $missing
 manager=$1; shift
 if [ "$(id -u)" -eq 0 ]; then
  as_root=''
 elif command -v sudo >/dev/null 2>&1; then
  as_root=sudo
 elif command -v doas >/dev/null 2>&1; then
  as_root=doas
 else
  fail "install these packages as root, then rerun with --skip-deps: $*"
 fi
 say "installing $* with $manager (requires administrator rights)"
 # Package managers must not read the piped script from standard input.
 case "$manager" in
  pacman) $as_root pacman -S --needed --noconfirm "$@" </dev/null ;;
  apt-get) $as_root apt-get update </dev/null && $as_root apt-get install -y --no-install-recommends "$@" </dev/null ;;
  zypper) $as_root zypper --non-interactive install "$@" </dev/null ;;
  dnf) $as_root dnf install -y "$@" </dev/null ;;
 esac || fail "package installation failed: $*"
}

# Prints the string value of "key":"..." in standard input, where it is unique.
json_string() { sed -n "s/.*\"$1\":\"\\([^\"]*\\)\".*/\\1/p"; }

# Verifies the signed feed and sets version, package_url, package_sha256,
# package_size and file_hashes ("name sha256" lines) for linux/amd64.
read_feed() {
 feed=$work/update-feed.json
 sed -n 's/^{"payload":"\([A-Za-z0-9+/=]*\)","signature":"[A-Za-z0-9+/=]*"}$/\1/p' "$feed" | openssl base64 -d -A > "$work/payload"
 sed -n 's/^{"payload":"[A-Za-z0-9+/=]*","signature":"\([A-Za-z0-9+/=]*\)"}$/\1/p' "$feed" | openssl base64 -d -A > "$work/signature"
 [ -s "$work/payload" ] && [ "$(wc -c < "$work/signature" | tr -d ' ')" -eq 64 ] || fail 'invalid signed update feed'
 # The raw key prefixed with the Ed25519 SubjectPublicKeyInfo header.
 printf '%s\n' '-----BEGIN PUBLIC KEY-----' "MCowBQYDK2VwAyEA$public_key" '-----END PUBLIC KEY-----' > "$work/key.pem"
 openssl pkeyutl -verify -pubin -inkey "$work/key.pem" -rawin -in "$work/payload" -sigfile "$work/signature" >/dev/null 2>&1 \
  || fail 'update feed signature verification failed'
 installer=$(sed -n 's/.*"installerUpdate":{\(.*\)}}$/\1/p' "$work/payload")
 [ -n "$installer" ] || fail 'this release predates the Linux installer; choose 0.1.6 or later'
 feed_version=$(printf '%s\n' "$installer" | sed -n 's/^"schema":2,"version":"\([^"]*\)",.*/\1/p')
 asset=$(printf '%s\n' "$installer" | grep -o '{"os":"linux","arch":"amd64",[^]]*]' | head -n 1)
 [ -n "$feed_version" ] && [ -n "$asset" ] || fail 'the release has no Linux amd64 package'
 [ -z "$version" ] || [ "$feed_version" = "$version" ] || fail "update feed is for $feed_version, not $version"
 version=$feed_version
 full=$(printf '%s\n' "$asset" | sed -n 's/.*"full":\({[^}]*}\).*/\1/p')
 package_url=$(printf '%s\n' "$full" | json_string url)
 package_sha256=$(printf '%s\n' "$full" | sed -n 's/.*"sha256":"\([0-9a-f]\{64\}\)".*/\1/p')
 package_size=$(printf '%s\n' "$full" | sed -n 's/.*"size":\([0-9]\{1,\}\).*/\1/p')
 [ "$(printf '%s\n' "$asset" | sed -n 's/.*"format":"\([^"]*\)".*/\1/p')" = tar-gzip ] || fail 'unexpected Linux package format'
 [ -n "$package_sha256" ] && [ -n "$package_size" ] || fail 'the update feed lacks the package checksum'
 file_hashes=''
 for name in passpine passpine-host; do
  hash=$(printf '%s\n' "$asset" | grep -o "{\"path\":\"$name\",[^}]*}" | sed -n 's/.*"sha256":"\([0-9a-f]\{64\}\)".*/\1/p')
  [ -n "$hash" ] || fail "the update feed lacks the checksum of $name"
  file_hashes="$file_hashes$name $hash
"
 done
}

main() {
 action=install
 version=''
 skip_deps=false
 while [ "$#" -gt 0 ]; do
  case "$1" in
   --version) [ "$#" -ge 2 ] || fail '--version needs a value'; version=${2#v}; shift ;;
   --version=*) version=${1#--version=}; version=${version#v} ;;
   --skip-deps) skip_deps=true ;;
   --uninstall) action=uninstall ;;
   -h|--help) usage; exit 0 ;;
   *) usage >&2; fail "unknown option: $1" ;;
  esac
  shift
 done
 [ "$(uname -s)" = Linux ] || fail 'this installer requires Linux; see the README for macOS and Windows'
 [ -n "${HOME:-}" ] || fail 'HOME is not set'
 if [ -n "$version" ]; then
  printf '%s\n' "$version" | grep -Eq '^[0-9]+\.[0-9]+\.[0-9]+([+-][0-9A-Za-z.+-]+)?$' || fail "invalid version: $version"
 fi

 bin_dir=${PASSPINE_BIN_DIR:-$HOME/.local/bin}
 data_home=${XDG_DATA_HOME:-$HOME/.local/share}
 for dir in "$bin_dir" "$data_home"; do
  case "$dir" in /*) ;; *) fail "installation directories must be absolute paths: $dir" ;; esac
  case "$dir" in *'"'*|*'`'*|*'$'*|*'\'*|*'%'*|*'
'*) fail "unsupported character in installation path: $dir" ;; esac
 done
 # install.sh records the resolved directory in the desktop entry.
 if [ -d "$bin_dir" ]; then bin_dir=$(cd "$bin_dir" && pwd -P); fi
 bin_dir=${bin_dir%/}
 desktop_path=${data_home%/}/applications/$app_id.desktop
 icon_path=${data_home%/}/icons/hicolor/256x256/apps/$app_id.png
 data_root=${PASSPINE_HOME:-${XDG_CONFIG_HOME:-$HOME/.config}/Passpine}
 for target in "$bin_dir/passpine" "$bin_dir/passpine-host" "$desktop_path" "$icon_path"; do
  case "$target/" in "${data_root%/}/"*) fail "installation overlaps the application data directory: $target" ;; esac
  [ ! -L "$target" ] || fail "refusing to replace or remove a symlink: $target"
  [ ! -e "$target" ] || [ -f "$target" ] || fail "not a regular file: $target"
 done
 # Only a release Passpine may be replaced or removed.
 for binary in passpine passpine-host; do
  [ -e "$bin_dir/$binary" ] || continue
  [ -x "$bin_dir/$binary" ] && [ "$("$bin_dir/$binary" --build-channel 2>/dev/null </dev/null)" = release ] \
   || fail "refusing to replace or remove an unrelated command: $bin_dir/$binary"
 done
 if [ -e "$desktop_path" ]; then
  grep -Fqx "Exec=\"$bin_dir/passpine\"" "$desktop_path" || fail "desktop entry belongs to another installation: $desktop_path"
 fi

 if [ "$action" = uninstall ]; then
  if [ -x "$bin_dir/passpine-host" ]; then
   "$bin_dir/passpine-host" --unregister </dev/null || fail 'cannot unregister the browser connection; nothing was removed'
  fi
  rm -f "$bin_dir/passpine" "$bin_dir/passpine-host" "$desktop_path" "$icon_path"
  if command -v update-desktop-database >/dev/null 2>&1; then update-desktop-database "${desktop_path%/*}" 2>/dev/null || true; fi
  say "removed $bin_dir/passpine, the browser connection, desktop entry and icon (if installed)"
  say 'vaults, private keys and settings have been kept; a running Passpine keeps working until it quits'
  exit 0
 fi

 [ "$(uname -m)" = x86_64 ] || fail "official Linux builds are x86_64 only; build from source on $(uname -m)"
 [ "$skip_deps" = true ] || install_dependencies
 for tool in openssl tar bash; do
  command -v "$tool" >/dev/null 2>&1 || fail "$tool is required"
 done

 work=$(mktemp -d "${TMPDIR:-/tmp}/passpine-install.XXXXXX")
 trap 'rm -rf "$work"' EXIT
 trap 'exit 1' HUP INT TERM

 if [ -n "$version" ]; then feed_url=$releases_url/download/v$version/update-feed.json; else feed_url=$releases_url/latest/download/update-feed.json; fi
 fetch "$feed_url" "$work/update-feed.json" || fail "cannot download the update feed: $feed_url"
 read_feed
 name=Passpine-$version-linux-amd64.tar.gz
 [ "$package_url" = "$official_releases_url/download/v$version/$name" ] || fail "unexpected package URL: $package_url"
 # Download from the configured source; the signed checksum still applies.
 say "downloading Passpine $version"
 fetch "$releases_url/download/v$version/$name" "$work/$name" || fail "download failed: $name"
 [ "$(wc -c < "$work/$name" | tr -d ' ')" = "$package_size" ] && [ "$(sha256 "$work/$name")" = "$package_sha256" ] \
  || fail "checksum mismatch: $name"
 mkdir "$work/release"
 tar -xzf "$work/$name" -C "$work/release" --no-same-owner passpine passpine-host install.sh app-icon-monochrome.png version.txt
 [ ! -e "$work/release/channel.txt" ] && [ "$(cat "$work/release/version.txt")" = "$version" ] || fail "unexpected package contents: $name"
 printf '%s' "$file_hashes" | while read -r binary hash; do
  [ "$(sha256 "$work/release/$binary")" = "$hash" ] || fail "checksum mismatch: $binary"
 done
 [ "$("$work/release/passpine-host" --build-channel </dev/null)" = release ] || fail 'downloaded package is not an official release'

 mkdir -p "$bin_dir"
 bash "$work/release/install.sh" "$bin_dir" </dev/null || fail 'installation failed'
 say "installed Passpine $version to $bin_dir/passpine"
 case ":${PATH:-}:" in
  *":$bin_dir:"*) ;;
  *) say "add $bin_dir to your shell PATH to run passpine from a terminal" ;;
 esac
 say 'later updates arrive in the app; restart a running Passpine to use this version'
 say 'uninstall: rerun this script with --uninstall (keeps vaults, private keys and settings)'
}

main "$@"
