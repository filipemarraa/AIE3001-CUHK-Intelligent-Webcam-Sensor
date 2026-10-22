#!/usr/bin/env bash
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"
assets="$root/packages/vitals-web/assets"
limit=1572864

mkdir -p "$assets"
(cd "$root/engine" && GOOS=js GOARCH=wasm go build -trimpath -ldflags="-s -w" -o "$assets/engine.wasm" ./cmd/wasm)

goroot="$(go env GOROOT)"
exec_js="$goroot/lib/wasm/wasm_exec.js"
[ -f "$exec_js" ] || exec_js="$goroot/misc/wasm/wasm_exec.js"
install -m 644 "$exec_js" "$assets/wasm_exec.js"

raw=$(wc -c < "$assets/engine.wasm")
compressed=$(node -e 'const z=require("node:zlib"),fs=require("node:fs");console.log(z.brotliCompressSync(fs.readFileSync(process.argv[1]),{params:{[z.constants.BROTLI_PARAM_QUALITY]:11}}).length)' "$assets/engine.wasm")
echo "engine.wasm: $raw bytes raw, $compressed bytes brotli (limit $limit)"
if [ "$compressed" -gt "$limit" ]; then
  echo "engine.wasm exceeds the brotli size limit" >&2
  exit 1
fi
