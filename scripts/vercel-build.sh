#!/usr/bin/env bash
# Production build on Vercel: the build image has no Go, so a pinned, checksum-verified Go is installed first.
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"
go_version=go1.27.3
go_sha256=65efbc80fdc1fa2cd2631cad5b481777a68e8725f34e89f50fc6ac714e379b40

if ! command -v go >/dev/null; then
  tmp="$(mktemp -d)"
  curl -fsSL "https://go.dev/dl/$go_version.linux-amd64.tar.gz" -o "$tmp/go.tgz"
  echo "$go_sha256  $tmp/go.tgz" | sha256sum -c -
  tar -C "$tmp" -xzf "$tmp/go.tgz"
  export PATH="$tmp/go/bin:$PATH"
fi

cd "$root"
npm run build:wasm --workspace packages/vitals-web
./scripts/fetch-assets.sh
npm run build --workspace packages/vitals-web
npm run build --workspace apps/demo
