#!/usr/bin/env bash
# Build a double-clickable macOS app: "TaleSpire Map Generator.app".
# Run this ON a Mac (needs the Go toolchain; `lipo` is used for a universal
# binary when available). Output goes to dist/.
set -euo pipefail
cd "$(dirname "$0")/.."

APP_NAME="TaleSpire Map Generator"
BIN_NAME="talespire-map-generator"
BUNDLE_ID="com.kirkanoskun.talespire-map-generator"
VERSION="1.0"

DIST="dist"
APP="$DIST/$APP_NAME.app"
MACOS="$APP/Contents/MacOS"
rm -rf "$APP"
mkdir -p "$MACOS"

echo "Building binaries..."
GOOS=darwin GOARCH=arm64 go build -trimpath -o "$DIST/${BIN_NAME}-arm64" ./cmd/server
GOOS=darwin GOARCH=amd64 go build -trimpath -o "$DIST/${BIN_NAME}-amd64" ./cmd/server

if command -v lipo >/dev/null 2>&1; then
  echo "Creating universal binary..."
  lipo -create -output "$MACOS/$BIN_NAME" "$DIST/${BIN_NAME}-arm64" "$DIST/${BIN_NAME}-amd64"
  rm -f "$DIST/${BIN_NAME}-arm64" "$DIST/${BIN_NAME}-amd64"
else
  echo "lipo not found; using the host-arch binary only."
  HOST_ARCH="$(uname -m)"; [ "$HOST_ARCH" = "x86_64" ] && HOST_ARCH="amd64" || HOST_ARCH="arm64"
  mv "$DIST/${BIN_NAME}-${HOST_ARCH}" "$MACOS/$BIN_NAME"
  rm -f "$DIST/${BIN_NAME}-arm64" "$DIST/${BIN_NAME}-amd64"
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
