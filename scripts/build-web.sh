#!/usr/bin/env bash
# Build the public website: a fully static site where the map engine runs in
# the visitor's browser (Go compiled to WebAssembly, see cmd/web). Needs only
# the Go toolchain. Output goes to dist/web/, ready for any static host — the
# deployment workflow publishes it to Cloudflare Pages.
#
# Usage: scripts/build-web.sh
set -euo pipefail
cd "$(dirname "$0")/.."

OUT="dist/web"
rm -rf "$OUT"
mkdir -p "$OUT/static"

echo "==> compiling the engine to WebAssembly"
GOOS=js GOARCH=wasm go build -trimpath -ldflags='-s -w' -o "$OUT/app.wasm" ./cmd/web

# wasm_exec.js is Go's JS runtime glue; it must come from the same toolchain
# that compiled app.wasm. Go 1.24 moved it from misc/wasm to lib/wasm.
GOROOT="$(go env GOROOT)"
for glue in "$GOROOT/lib/wasm/wasm_exec.js" "$GOROOT/misc/wasm/wasm_exec.js"; do
  if [[ -f "$glue" ]]; then cp "$glue" "$OUT/"; break; fi
done
[[ -f "$OUT/wasm_exec.js" ]] || { echo "error: wasm_exec.js not found under $GOROOT" >&2; exit 1; }

echo "==> assembling the site"
cp web/cartographe.js web/_headers "$OUT/"
# Everything the desktop app serves under /static/ (logos, fonts, legal page);
# index.html is assembled separately below.
cp -R internal/server/static/. "$OUT/static/"
rm "$OUT/static/index.html"

# The page is the desktop app's own index.html, with the engine loaded before
# the UI's script so the UI's first request is already routed to it.
awk '
  !done && $0 == "<script>" {
    print "<script src=\"/wasm_exec.js\"></script>"
    print "<script src=\"/cartographe.js\"></script>"
    done = 1
  }
  { print }
  END { if (!done) exit 1 }
' internal/server/static/index.html > "$OUT/index.html" \
  || { echo "error: no <script> line in index.html to load the engine before" >&2; exit 1; }

echo "==> done: $OUT"
ls -l "$OUT" | awk 'NR > 1 { printf "    %-16s %8.1f kB\n", $NF, $5 / 1024 }'
