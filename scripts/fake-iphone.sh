#!/bin/bash
# 시험용 가짜 iPhone 폴더 만들기: scripts/fake-iphone.sh <만들 폴더>
# scripts/testdata 의 사진을 복사하고, dates.txt 에 적힌 찍은 날짜를 파일 날짜로 맞춰요 (맥 · 리눅스 모두)
set -euo pipefail
ROOT="$(cd "$(dirname "$0")" && pwd)/testdata"
DEST="$1"
rm -rf "$DEST"
mkdir -p "$DEST/DCIM/100APPLE"
cp "$ROOT/DCIM/100APPLE/"* "$DEST/DCIM/100APPLE/"
while read -r stamp name; do
  [ -n "$name" ] && touch -t "$stamp" "$DEST/DCIM/100APPLE/$name"
done < "$ROOT/dates.txt"
echo "iPhone" > "$DEST/.name"
echo "✔ 가짜 iPhone: $DEST ($(ls "$DEST/DCIM/100APPLE" | wc -l | tr -d ' ')개 파일)"
