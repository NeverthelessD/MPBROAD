#!/bin/bash
# MyPhotoBook_ROAD 맥 빌드 (맥에서만 돌아요 — 맥 빌드 서버 scripts/ci-mac.sh 가 불러요)
#   Go 엔진(darwin/arm64) + Swift 앱 껍데기(app/shell) + Swift iPhone 도우미(app/device) → dist/MyPhotoBook_ROAD.app
#   → 임시 서명(ad-hoc) → dist/MPBROAD-<버전>.zip (업데이트용) · dist/MyPhotoBook_ROAD.zip (처음 설치용, 사용 가이드 포함)
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
VERSION="$(tr -d ' \n' < "$ROOT/VERSION")"
NAME="MyPhotoBook_ROAD"
DIST="$ROOT/dist"
APP="$DIST/$NAME.app"
GUIDE="$ROOT/docs/${NAME}_사용가이드.png"
rm -rf "$DIST"
mkdir -p "$APP/Contents/MacOS" "$APP/Contents/Resources"

echo "▶ 엔진 시험 · 빌드 (darwin/arm64, v$VERSION)"
( cd "$ROOT/engine" && go test -count=1 ./... )
( cd "$ROOT/engine" && CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 \
    go build -trimpath -ldflags "-s -w -X main.build=$VERSION" -o "$APP/Contents/Resources/mpbroad-engine" . )
chmod 755 "$APP/Contents/Resources/mpbroad-engine"

echo "▶ Swift 빌드"
xcrun swiftc --version
xcrun --show-sdk-version || true
echo "  · 앱 껍데기"
xcrun swiftc -O -target arm64-apple-macos12.0 \
  -o "$APP/Contents/MacOS/$NAME" "$ROOT/app/shell/main.swift" \
  -framework AppKit -framework WebKit
echo "  · iPhone 도우미 (ImageCaptureCore)"
xcrun swiftc -O -target arm64-apple-macos12.0 \
  -o "$APP/Contents/Resources/mpbroad-device" "$ROOT/app/device/main.swift" \
  -framework ImageCaptureCore -framework ImageIO -framework CoreGraphics
chmod 755 "$APP/Contents/MacOS/$NAME" "$APP/Contents/Resources/mpbroad-device"

sed "s/__VERSION__/$VERSION/g" "$ROOT/app/Info.plist" > "$APP/Contents/Info.plist"
plutil -lint "$APP/Contents/Info.plist"
cp "$ROOT/app/AppIcon.icns" "$APP/Contents/Resources/AppIcon.icns"
printf '%s' "$VERSION" > "$APP/Contents/Resources/version.txt"
printf 'APPL????' > "$APP/Contents/PkgInfo"

echo "▶ 임시 서명 (ad-hoc) · 확인"
codesign --force --sign - --timestamp=none "$APP/Contents/Resources/mpbroad-engine"
codesign --force --sign - --timestamp=none "$APP/Contents/Resources/mpbroad-device"
codesign --force --deep --sign - --timestamp=none "$APP"
codesign --verify --deep --strict --verbose=2 "$APP"
codesign -dv "$APP" 2>&1 | sed 's/^/  /'

echo "▶ 압축"
ditto -c -k --norsrc --keepParent "$APP" "$DIST/MPBROAD-$VERSION.zip"
mkdir -p "$DIST/first/$NAME"
ditto "$APP" "$DIST/first/$NAME/$NAME.app"
[ -f "$GUIDE" ] && cp "$GUIDE" "$DIST/first/$NAME/"
ditto -c -k --norsrc --keepParent "$DIST/first/$NAME" "$DIST/$NAME.zip"

echo "▶ 업데이트 파일을 풀어도 서명이 그대로인지 확인 (앱의 업데이트 기능처럼 unzip)"
CHECK="$DIST/check"; rm -rf "$CHECK"; mkdir -p "$CHECK"
( cd "$CHECK" && unzip -q "$DIST/MPBROAD-$VERSION.zip" )
codesign --verify --deep --strict "$CHECK/$NAME.app"
[ "$(cat "$CHECK/$NAME.app/Contents/Resources/version.txt")" = "$VERSION" ]
rm -rf "$CHECK"
echo "✔ dist/MPBROAD-$VERSION.zip · dist/$NAME.zip"
