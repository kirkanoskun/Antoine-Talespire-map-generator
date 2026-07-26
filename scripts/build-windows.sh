#!/usr/bin/env bash
# Build a Windows executable with no console window:
# "TaleSpire Map Generator.exe". Can be cross-compiled from any OS with Go.
# Output goes to dist/.
set -euo pipefail
cd "$(dirname "$0")/.."

mkdir -p dist
echo "Building Windows .exe (no console window)..."
GOOS=windows GOARCH=amd64 go build -trimpath \
  -ldflags "-H=windowsgui" \
  -o "dist/TaleSpire Map Generator.exe" ./cmd/server

echo "Built: dist/TaleSpire Map Generator.exe"
echo
echo "Double-click it: it opens the app in your default browser. Put your"
echo "Anthropic key next to the .exe in a file named .env, or in"
echo "%USERPROFILE%\\.talespire\\.env :"
echo "  ANTHROPIC_API_KEY=sk-ant-..."
