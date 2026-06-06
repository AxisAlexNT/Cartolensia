#!/usr/bin/env bash
set -euo pipefail

ROOT="${CARTOLENSIA_SYNTHETIC_ROOT:-testdata/synthetic_media}"
FOLDERS="${CARTOLENSIA_SYNTHETIC_FOLDERS:-10}"
FILES_PER_FOLDER="${CARTOLENSIA_SYNTHETIC_FILES_PER_FOLDER:-25}"
DUPLICATE_RATE="${CARTOLENSIA_SYNTHETIC_DUPLICATE_RATE:-5}"

mkdir -p "$ROOT"
printf 'synthetic duplicate seed\n' > "$ROOT/duplicate-seed.jpg"

for folder in $(seq 1 "$FOLDERS"); do
  folder_path="$ROOT/set_$(printf '%03d' "$folder")"
  mkdir -p "$folder_path/photos" "$folder_path/videos" "$folder_path/tracks"
  for file in $(seq 1 "$FILES_PER_FOLDER"); do
    idx="$(printf '%03d_%04d' "$folder" "$file")"
    mod=$((file % 10))
    if [ "$DUPLICATE_RATE" -gt 0 ] && [ $((file % DUPLICATE_RATE)) -eq 0 ]; then
      cp "$ROOT/duplicate-seed.jpg" "$folder_path/photos/duplicate_$idx.jpg"
    elif [ "$mod" -lt 6 ]; then
      {
        echo "synthetic image-like fixture"
        echo "taken_at_hint=2024-06-01T10:00:00Z"
      } > "$folder_path/photos/photo_$idx.jpg"
    elif [ "$mod" -lt 8 ]; then
      {
        echo "synthetic video-like fixture"
        echo "taken_at_hint=2024-06-01T10:00:00Z"
        echo "duration_hint_seconds=60"
      } > "$folder_path/videos/clip_$idx.mp4"
    else
      cat > "$folder_path/tracks/track_$idx.gpx" <<GPX
<?xml version="1.0" encoding="UTF-8"?>
<gpx version="1.1" creator="cartolensia-synthetic">
  <trk><name>track_$idx</name><trkseg>
    <trkpt lat="40.$folder" lon="44.$file"><ele>100</ele><time>2024-06-01T10:00:00Z</time></trkpt>
    <trkpt lat="40.$folder" lon="44.$((file + 1))"><ele>110</ele><time>2024-06-01T10:05:00Z</time></trkpt>
  </trkseg></trk>
</gpx>
GPX
    fi
  done
done

find "$ROOT" -type f | wc -l | awk '{print "synthetic files: "$1}'
