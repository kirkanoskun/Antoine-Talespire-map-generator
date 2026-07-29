#!/usr/bin/env bash
# Build a double-clickable macOS app: "TaleSpire Map Generator.app".
# Run this ON a Mac (needs the Go toolchain). Output goes to dist/.
#
# By default it builds ONLY for the current machine's architecture (detected via
# `uname -m`), so it needs nothing but the Go toolchain — no Xcode Command Line
# Tools, no `lipo`. Pass --universal to instead build a fat Intel+Apple-Silicon
# binary; that path requires `lipo` (from the Command Line Tools).
#
# Usage: scripts/build-mac.sh [--universal]
set -euo pipefail
cd "$(dirname "$0")/.."

UNIVERSAL=0
for arg in "$@"; do
  case "$arg" in
    --universal) UNIVERSAL=1 ;;
    -h|--help)
      echo "Usage: scripts/build-mac.sh [--universal]"
      echo "  (default)     build for this machine's architecture only (no lipo needed)"
      echo "  --universal   build a universal Intel+Apple-Silicon binary (needs lipo)"
      exit 0 ;;
    *) echo "Unknown argument: $arg" >&2; echo "Usage: scripts/build-mac.sh [--universal]" >&2; exit 2 ;;
  esac
done

APP_NAME="TaleSpire Map Generator"
BIN_NAME="talespire-map-generator"
BUNDLE_ID="com.kirkanoskun.talespire-map-generator"
VERSION="1.0"

DIST="dist"
APP="$DIST/$APP_NAME.app"
MACOS="$APP/Contents/MacOS"
rm -rf "$APP"
mkdir -p "$MACOS"

if [ "$UNIVERSAL" -eq 1 ]; then
  if ! command -v lipo >/dev/null 2>&1; then
    echo "error: --universal needs 'lipo' (Xcode Command Line Tools), which was not found." >&2
    echo "       Run without --universal to build for this machine's architecture only." >&2
    exit 1
  fi
  echo "Building universal binary (arm64 + amd64)..."
  GOOS=darwin GOARCH=arm64 go build -trimpath -o "$DIST/${BIN_NAME}-arm64" ./cmd/server
  GOOS=darwin GOARCH=amd64 go build -trimpath -o "$DIST/${BIN_NAME}-amd64" ./cmd/server
  lipo -create -output "$MACOS/$BIN_NAME" "$DIST/${BIN_NAME}-arm64" "$DIST/${BIN_NAME}-amd64"
  rm -f "$DIST/${BIN_NAME}-arm64" "$DIST/${BIN_NAME}-amd64"
else
  # Detect the host architecture and build for it alone — no lipo, no
  # cross-compile of the other arch.
  case "$(uname -m)" in
    arm64|aarch64) HOST_ARCH="arm64" ;;
    x86_64|amd64)  HOST_ARCH="amd64" ;;
    *) echo "error: unsupported architecture '$(uname -m)'." >&2; exit 1 ;;
  esac
  echo "Building for this machine's architecture ($HOST_ARCH)..."
  GOOS=darwin GOARCH="$HOST_ARCH" go build -trimpath -o "$MACOS/$BIN_NAME" ./cmd/server
fi
chmod +x "$MACOS/$BIN_NAME"

cat > "$APP/Contents/Info.plist" <<PLIST
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>CFBundleName</key><string>$APP_NAME</string>
  <key>CFBundleDisplayName</key><string>$APP_NAME</string>
  <key>CFBundleIdentifier</key><string>$BUNDLE_ID</string>
  <key>CFBundleVersion</key><string>$VERSION</string>
  <key>CFBundleShortVersionString</key><string>$VERSION</string>
  <key>CFBundleExecutable</key><string>$BIN_NAME</string>
  <key>CFBundlePackageType</key><string>APPL</string>
  <key>LSMinimumSystemVersion</key><string>10.13</string>
  <key>NSHighResolutionCapable</key><true/>
</dict>
</plist>
PLIST

echo "Built: $APP"
echo
echo "First launch: right-click the app -> Open (once) to bypass Gatekeeper,"
echo "since the app is unsigned. Put your Anthropic key in ~/.talespire/.env:"
echo "  mkdir -p ~/.talespire && echo 'ANTHROPIC_API_KEY=sk-ant-...' > ~/.talespire/.env"
