#!/usr/bin/env bash
# Renders the promo in every locale, plus a dedicated poster frame, into out/.
# Pass Remotion flags through, e.g. --browser-executable=/usr/bin/google-chrome-stable.
set -euo pipefail
cd "$(dirname "$0")/.."

pnpm run prepare:assets
mkdir -p out

for locale in zh-CN en; do
  name="passpine-promo-${locale}"
  pnpm exec remotion render src/index.ts "Promo-${locale}" "out/${name}.raw.mp4" "$@"
  # Move the moov atom to the front so browsers can start playback before download finishes.
  pnpm exec remotion ffmpeg -loglevel error -y -i "out/${name}.raw.mp4" -c copy -movflags +faststart "out/${name}.mp4"
  rm "out/${name}.raw.mp4"
  pnpm exec remotion still src/index.ts "Poster-${locale}" "out/${name}.jpg" --image-format=jpeg --jpeg-quality=90 "$@"
done
