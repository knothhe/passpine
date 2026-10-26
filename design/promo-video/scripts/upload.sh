#!/usr/bin/env bash
# Uploads the rendered promo (see render.sh) to the R2 bucket behind oss.knothhe.com.
# Requires `pnpm --dir website exec wrangler login`. Bump VERSION for new cuts and update
# website/src/constants/media.ts; existing versions are never overwritten.
set -euo pipefail
cd "$(dirname "$0")/.."

BUCKET=oss
VERSION=v1
PREFIX="passpine/promo/${VERSION}"
ORIGIN=https://oss.knothhe.com
wrangler() { pnpm --dir ../../website exec wrangler "$@"; }

for file in out/passpine-promo-{zh-CN,en}.{mp4,jpg}; do
  key="${PREFIX}/$(basename "$file")"
  if [ "$(curl -s -o /dev/null -w '%{http_code}' -I "${ORIGIN}/${key}")" != 404 ]; then
    echo "skip ${key}: already exists" >&2
    continue
  fi
  case "$file" in
    *.mp4) type=video/mp4 ;;
    *.jpg) type=image/jpeg ;;
  esac
  wrangler r2 object put "${BUCKET}/${key}" --remote --file "$(pwd)/${file}" \
    --content-type "$type" --cache-control "public, max-age=31536000, immutable"
done
