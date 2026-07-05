#!/usr/bin/env bash
# Build Claude Migrator for Windows + macOS (universal). Requires Go 1.22+.
set -e
cd "$(dirname "$0")/.."
mkdir -p dist
export CGO_ENABLED=0
VER=$(grep -oE 'VERSION = "[0-9.]+"' main.go | grep -oE '[0-9.]+')

command -v go-winres >/dev/null && go-winres make --in winres/winres.json || true
echo "windows/amd64..."; GOOS=windows GOARCH=amd64 go build -ldflags="-s -w -H windowsgui" -o dist/ClaudeMigrator-win.exe .
echo "darwin/arm64...";  GOOS=darwin  GOARCH=arm64 go build -ldflags="-s -w" -o dist/cm_arm64 .
echo "darwin/amd64...";  GOOS=darwin  GOARCH=amd64 go build -ldflags="-s -w" -o dist/cm_amd64 .

# universal macOS binary
command -v makefat >/dev/null 2>&1 || GOBIN="$PWD/dist" go install github.com/randall77/makefat@latest
MAKEFAT=$(command -v makefat || echo dist/makefat)
"$MAKEFAT" dist/cm_universal dist/cm_amd64 dist/cm_arm64

# .app bundle -> tar.gz (tar preserves the +x bit)
APP=dist/ClaudeMigrator.app; rm -rf "$APP"; mkdir -p "$APP/Contents/MacOS" "$APP/Contents/Resources"
cp assets/ClaudeMigrator.icns "$APP/Contents/Resources/" 2>/dev/null || true
cp dist/cm_universal "$APP/Contents/MacOS/ClaudeMigrator"; chmod +x "$APP/Contents/MacOS/ClaudeMigrator"
cat > "$APP/Contents/Info.plist" <<PLIST
<?xml version="1.0" encoding="UTF-8"?>
<plist version="1.0"><dict>
<key>CFBundleName</key><string>ClaudeMigrator</string>
<key>CFBundleExecutable</key><string>ClaudeMigrator</string>
<key>CFBundleIdentifier</key><string>com.finessed.claudemigrator</string>
<key>CFBundleVersion</key><string>${VER}</string>
<key>CFBundleShortVersionString</key><string>${VER}</string>
<key>CFBundlePackageType</key><string>APPL</string>
<key>CFBundleIconFile</key><string>ClaudeMigrator</string>
<key>NSHighResolutionCapable</key><true/>
<key>LSUIElement</key><true/>
</dict></plist>
PLIST
( cd dist && tar -czf ClaudeMigrator-mac.tar.gz ClaudeMigrator.app )
rm -f dist/cm_arm64 dist/cm_amd64 dist/cm_universal dist/makefat
echo "Built v${VER} -> dist/ClaudeMigrator-win.exe, dist/ClaudeMigrator-mac.tar.gz"
