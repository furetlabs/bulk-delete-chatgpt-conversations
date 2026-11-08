#!/bin/sh
set -eu
[ "$(uname -s)" = Darwin ] || { echo 'macOS and Xcode command-line tools are required' >&2; exit 1; }
cd "$(dirname "$0")/.."
output="${1:-dist/ChatGPT Cleaner.app}"
[ ! -e "$output" ] || { echo "Refusing to overwrite $output" >&2; exit 1; }
mkdir -p "$(dirname "$output")"
staging=$(mktemp -d "$(dirname "$output")/.chatgpt-cleaner.XXXXXX")
trap 'rm -rf "$staging"' EXIT HUP INT TERM
bundle="$staging/ChatGPT Cleaner.app"
mkdir -p "$bundle/Contents/MacOS" "$bundle/Contents/Resources" "$staging/AppIcon.iconset"
CGO_ENABLED=1 GOOS=darwin GOARCH=arm64 go build -mod=readonly -o "$bundle/Contents/MacOS/chatgpt-cleaner" ./cmd/chatgpt-cleaner
for size in 16 32 128 256 512; do
  sips -z "$size" "$size" assets/transparent-background.png --out "$staging/AppIcon.iconset/icon_${size}x${size}.png" >/dev/null
  double=$((size * 2))
  sips -z "$double" "$double" assets/transparent-background.png --out "$staging/AppIcon.iconset/icon_${size}x${size}@2x.png" >/dev/null
done
iconutil -c icns "$staging/AppIcon.iconset" -o "$bundle/Contents/Resources/AppIcon.icns"
cp LICENSE "$bundle/Contents/Resources/LICENSE.txt"
cat > "$bundle/Contents/Info.plist" <<'PLIST'
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
<key>CFBundleName</key><string>ChatGPT Cleaner</string>
<key>CFBundleDisplayName</key><string>ChatGPT Cleaner</string>
<key>CFBundleIdentifier</key><string>org.furetlabs.chatgptcleaner</string>
<key>CFBundleExecutable</key><string>chatgpt-cleaner</string>
<key>CFBundleIconFile</key><string>AppIcon</string>
<key>CFBundlePackageType</key><string>APPL</string>
<key>CFBundleShortVersionString</key><string>0.1.0</string>
<key>CFBundleVersion</key><string>1</string>
<key>LSMinimumSystemVersion</key><string>11.0</string>
<key>NSHighResolutionCapable</key><true/>
</dict></plist>
PLIST
[ ! -e "$output" ] || { echo "Output appeared during build; refusing overwrite" >&2; exit 1; }
mv "$bundle" "$output"
printf 'Created %s (unsigned; not notarized)\n' "$output"
