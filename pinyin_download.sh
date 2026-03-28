#!/usr/bin/env bash

# ── Config ────────────────────────────────────────────────────────────────────
SOURCE_DIR="data/pinyin-audio"
DOMAIN="https://cdn.alternative.com"
URL_PATH="/"        # Subpath before the filename (no trailing slash)
OUTPUT_DIR="data/alternative"
# ─────────────────────────────────────────────────────────────────────────────

mkdir -p "$OUTPUT_DIR"

for filepath in "$SOURCE_DIR"/*.mp3; do
    [[ -e "$filepath" ]] || { echo "No .mp3 files found in $SOURCE_DIR"; exit 1; }

    filename=$(basename "$filepath")
    dest="$OUTPUT_DIR/$filename"

    if [[ -f "$dest" ]]; then
        echo "Skipping (exists): $filename"
        continue
    fi

    url="$DOMAIN$URL_PATH/$filename"
    echo "Downloading: $url"
    curl -fL --retry 3 -o "$dest" "$url" || echo "  ✗ Failed: $url"
done

diff <(ls "$SOURCE_DIR"/*.mp3 | xargs -n1 basename) <(ls "$OUTPUT_DIR"/*.mp3 | xargs -n1 basename)

echo "Done."
