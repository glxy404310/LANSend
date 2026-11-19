#!/bin/bash
# Run on macOS with Go 1.27.4+ and the Xcode Command Line Tools.
set -eu
cd "$(dirname "$0")/.."
out="${LANSEND_BUILD_DIR:-$PWD/build}"
app="$out/LANSend.app/Contents"
res="$app/Resources"

mkdir -p "$app/MacOS" "$res"
cp mac-desktop/Info.plist "$app/Info.plist"
cp mac-desktop/AppIcon.icns mac-desktop/Setup.sh "$res/"
ditto --noextattr --norsrc 'mac-desktop/发送到 Windows.workflow' "$res/发送到 Windows.workflow"
for arch in amd64 arm64; do
  CGO_ENABLED=0 GOOS=darwin GOARCH="$arch" go build -trimpath -ldflags='-s -w' -o "$out/core-$arch" .
done
lipo -create "$out/core-amd64" "$out/core-arm64" -output "$res/engine"
codesign --force --sign - "$res/engine"
xcrun clang -fobjc-arc -arch x86_64 -arch arm64 -mmacosx-version-min=11.0 -framework Cocoa mac-desktop/main.m -o "$app/MacOS/LANSend"
xattr -cr "$out/LANSend.app"
codesign --force --sign - "$out/LANSend.app"
