#!/usr/bin/env bash
# Checks that a live site serves exactly the files of a build: the page, the
# bridge, the prompt head and the engine, byte for byte. The deployment
# workflow runs it right after publishing, and rolls back if it fails.
#
# Usage: scripts/check-site.sh <url> [build-dir]
#   CHECK_ATTEMPTS (default 12) and CHECK_DELAY seconds (default 10) bound how
#   long it waits for the new version to be served everywhere.
set -euo pipefail

url="${1:?usage: scripts/check-site.sh <url> [build-dir]}"
url="${url%/}"
dir="${2:-dist/web}"
attempts="${CHECK_ATTEMPTS:-12}"
delay="${CHECK_DELAY:-10}"
files=(index.html cartographe.js wasm_exec.js prompt-head.txt app.wasm)

for f in "${files[@]}"; do
  [[ -f "$dir/$f" ]] || { echo "error: $dir/$f is missing; build the site first" >&2; exit 2; }
done

for try in $(seq 1 "$attempts"); do
  stale=""
  for f in "${files[@]}"; do
    path="/$f"
    [[ "$f" == index.html ]] && path="/"
    want="$(sha256sum < "$dir/$f" | cut -d' ' -f1)"
    got="$(curl -fsS --compressed --max-time 120 "$url$path" | sha256sum | cut -d' ' -f1)" || got="unreachable"
    [[ "$want" == "$got" ]] || stale+=" $f"
  done
  if [[ -z "$stale" ]]; then
    echo "ok: $url serves this build"
    exit 0
  fi
  echo "attempt $try/$attempts: not this build yet:$stale"
  [[ "$try" == "$attempts" ]] || sleep "$delay"
done
echo "error: $url does not serve this build" >&2
exit 1
