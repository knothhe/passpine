#!/usr/bin/env bash
# Renders the promo in every locale, plus a poster frame, into out/.
# Pass Remotion flags through, e.g. --browser-executable=/usr/bin/google-chrome-stable.
set -euo pipefail
cd "$(dirname "$0")/.."

node scripts/music.mjs
mkdir -p out
for locale in zh-CN en; do
  name="passpine-promo-${locale}"
  pnpm exec remotion render src/index.ts "Promo-${locale}" "out/${name}.raw.mp4" "$@"
  # Move the index to the front so browsers can start playback before the download finishes.
  ffmpeg -loglevel error -y -i "out/${name}.raw.mp4" -c copy -movflags +faststart "out/${name}.mp4"
  rm "out/${name}.raw.mp4"
  pnpm exec remotion still src/index.ts "Promo-${locale}" "out/${name}.jpg" --frame=175 --image-format=jpeg --jpeg-quality=85 "$@"
done
