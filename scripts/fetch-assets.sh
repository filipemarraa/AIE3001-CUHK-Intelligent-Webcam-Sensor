#!/usr/bin/env bash
# Downloads the MediaPipe models and copies the tasks-vision wasm runtime into the vitals-web assets.
# Every file is checked against assets.sha256; --update rewrites that file instead (only after a deliberate bump).
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"
dest="packages/vitals-web/assets/mediapipe"
sums="assets.sha256"
models="https://storage.googleapis.com/mediapipe-models"
face_url="$models/face_landmarker/face_landmarker/float16/latest/face_landmarker.task"
pose_url="$models/pose_landmarker/pose_landmarker_lite/float16/latest/pose_landmarker_lite.task"

cd "$root"
mkdir -p "$dest/wasm"

download() {
  local url="$1" out="$2"
  [ -f "$out" ] && return 0
  curl -fsSL --retry 3 -o "$out.part" "$url"
  mv "$out.part" "$out"
}

download "$face_url" "$dest/face_landmarker.task"
download "$pose_url" "$dest/pose_landmarker_lite.task"
cp node_modules/@mediapipe/tasks-vision/wasm/* "$dest/wasm/"

if [ "${1:-}" = "--update" ]; then
  find "$dest" -type f | LC_ALL=C sort | xargs shasum -a 256 > "$sums"
  echo "wrote $sums"
fi
shasum -a 256 -c "$sums"
