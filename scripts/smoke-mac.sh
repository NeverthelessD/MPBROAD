#!/bin/bash
# MyPhotoBook_ROAD 실제 맥 시험 (맥 빌드 서버에서 dist/MyPhotoBook_ROAD.app 을 /Applications 에 깔고 여러 상황을 시험해요)
# 빌드 서버에는 iPhone 이 없어서: A~C 는 진짜 iPhone 도우미(ImageCaptureCore)가 '연결된 iPhone 없음'으로 잘 도는지,
# I 는 가짜 iPhone 폴더(Support/test-fake-device)로 가져오기 · 정리 · 갑자기 꺼짐 → 이어서 하기 · 지우기를 시험해요.
# 결과: $OUT/results.txt (PASS/FAIL 한 줄씩), 스크린샷, 기록 파일
set -uo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
OUT="${OUT:-$ROOT/ci-out}"
mkdir -p "$OUT"
NAME="MyPhotoBook_ROAD"
APPDIR="/Applications/$NAME.app"
SUP="$HOME/Library/Application Support/$NAME"
RES="$OUT/results.txt"
: > "$RES"

pass() { echo "PASS  $*" | tee -a "$RES"; }
fail() { echo "FAIL  $*" | tee -a "$RES"; }
check() { local name="$1"; shift; if "$@"; then pass "$name"; else fail "$name"; fi; }

app_pid() { pgrep -x "$NAME" | head -1; }
engines() { pgrep -x mpbroad-engine | tr '\n' ' '; }
helpers() { pgrep -x mpbroad-device | tr '\n' ' '; }
tok() { cat "$SUP/token" 2>/dev/null; }
port() { cat "$SUP/port" 2>/dev/null; }
api() { curl -fs -m 5 -H "x-token: $(tok)" "http://127.0.0.1:$(port)$1"; }
post() { local body='{}'; [ $# -ge 2 ] && body="$2"; curl -fs -m 10 -X POST -H "x-token: $(tok)" -H 'content-type: application/json' --data "$body" "http://127.0.0.1:$(port)$1"; }
ping_ok() { api /api/ping >/dev/null 2>&1; }
wait_ready() { for _ in $(seq 1 60); do [ -s "$SUP/port" ] && ping_ok && return 0; sleep 0.5; done; return 1; }
wait_gone() { for _ in $(seq 1 30); do [ -z "$(app_pid)" ] && [ -z "$(engines)" ] && [ -z "$(helpers)" ] && return 0; sleep 0.5; done; return 1; }
snap() { echo "--- $1"; ps -axo pid,ppid,stat,comm | grep -E "$NAME|mpbroad" | grep -v grep; } >> "$OUT/procs.txt"
launch() { open "$APPDIR"; wait_ready; }
pyget() { python3 -c "import json,sys; d=json.load(sys.stdin); print($1)" 2>/dev/null; }
wait_stage() { for _ in $(seq 1 "${2:-120}"); do [ "$(api /api/poll | pyget "d['job'].get('stage','')")" = "$1" ] && return 0; sleep 0.5; done; return 1; }

echo "▶ 준비: 앱 설치"
rm -rf "$APPDIR"; ditto "$ROOT/dist/$NAME.app" "$APPDIR"
rm -rf "$SUP"; mkdir -p "$SUP"
printf '{"appearance":"light","autoUpdate":false,"destMode":"default","skipDone":true,"keepLive":true,"askDelete":true,"resumeJobs":true,"autoResume":true,"previewDefault":true,"delAfter":true}' > "$SUP/settings.json"

echo "▶ A. 켜기 · 화면 · 진짜 iPhone 도우미"
launch; check "A1 앱을 열면 엔진이 준비됨" ping_ok
check "A2 엔진이 Dock 앱 방식으로 켜짐(shell=native)" bash -c "curl -fs -H 'x-token: $(tok)' http://127.0.0.1:$(port)/api/ping | grep -q '\"shell\":\"native\"'"
check "A3 주소가 47931" test "$(port)" = "47931"
sleep 6
DEV=$(api /api/poll | pyget "d['device']['helper']+'/'+d['device']['state']")
check "A4 iPhone 도우미(ImageCaptureCore)가 켜져 기다림 ($DEV)" bash -c "[ '$DEV' = 'ok/none' ] || [ '$DEV' = 'ok/connecting' ]"
screencapture -x "$OUT/shot-app.png"
check "A5 화면이 앱 창(WKWebView)에서 열림" grep -q "MPBROADShell" "$SUP/requests.log"
APID=$(app_pid); snap "A 실행 중"
SUPPID=$(ps -axo pid=,ppid=,comm= | awk -v a="$APID" '$2==a && /mpbroad-engine/ {print $1}' | head -1)
CHILD=$(ps -axo pid=,ppid=,comm= | awk -v s="$SUPPID" '$2==s && /mpbroad-engine/ {print $1}' | head -1)
HELPER=$(ps -axo pid=,ppid=,comm= | awk -v s="$CHILD" '$2==s && /mpbroad-device/ {print $1}' | head -1)
check "A6 앱 $APID → 지킴이 ${SUPPID:-없음} → 엔진 ${CHILD:-없음} → iPhone 도우미 ${HELPER:-없음}" test -n "$SUPPID" -a -n "$CHILD" -a -n "$HELPER"
cp "$SUP/device.log" "$OUT/device-A.log" 2>/dev/null

echo "▶ B. Dock 아이콘 다시 누르기(reopen)"
osascript -e "tell application \"$NAME\" to reopen" >/dev/null 2>&1; sleep 1
check "B1 다시 열기 뒤에도 앱·엔진 그대로" test "$(app_pid)" = "$APID"

echo "▶ C. 엔진 · 도우미가 고장으로 꺼지면 다시 켬"
kill -SEGV "$HELPER" 2>/dev/null; sleep 4
check "C1 iPhone 도우미 다시 켜짐" bash -c "[ -n \"\$(pgrep -x mpbroad-device)\" ] && [ \"\$(pgrep -x mpbroad-device)\" != '$HELPER' ]"
kill -SEGV "$CHILD" 2>/dev/null; sleep 4
check "C2 엔진 다시 켜짐" ping_ok
check "C3 앱은 그대로" test "$(app_pid)" = "$APID"
osascript -e "tell application \"$NAME\" to quit" >/dev/null 2>&1
check "C4 종료(⌘Q) → 앱 · 엔진 · 도우미 모두 꺼짐" wait_gone

echo "▶ I. 가져오기 (가짜 iPhone): 연/월/일 정리 · 갑자기 꺼짐 → 이어서 하기 · 지우기"
PHONE="$HOME/MPB-test-iphone"; DEST="$HOME/MPB-test-dest"; rm -rf "$DEST"
bash "$ROOT/scripts/fake-iphone.sh" "$PHONE"
printf '%s' "$PHONE" > "$SUP/test-fake-device"
mkdir -p "$DEST"
python3 - "$SUP/settings.json" "$DEST" <<'PY'
import json, sys
p, d = sys.argv[1], sys.argv[2]
s = json.load(open(p)); s.update(destMode="custom", customDest=d); json.dump(s, open(p, "w"))
PY
launch
sleep 3
N=$(api /api/items | pyget "d['counts']['all']")
check "I1 가짜 iPhone 사진 30개 묶음 (읽은 수 ${N:-?})" test "${N:-0}" = "30"
sleep 3
screencapture -x "$OUT/shot-main.png"
echo 150000 > "$PHONE/.rate"
post /api/import '{"all":true,"askDelete":true}' > /dev/null
sleep 4
screencapture -x "$OUT/shot-importing.png"
check "I2 가져오는 중" wait_stage running 5
kill -9 "$(app_pid)"; sleep 3
check "I3 앱이 강제로 꺼지면 엔진 · 도우미도 꺼짐" wait_gone
rm -f "$PHONE/.rate"
launch; sleep 2
ST=$(api /api/poll | pyget "d['job']['stage']+'/'+str(d['job'].get('doneFiles'))+'/'+str(d['job'].get('total'))")
check "I4 다시 열면 '지난번 가져오기가 끝나지 않았어요' ($ST)" bash -c "[[ '$ST' == interrupted/* ]]"
sleep 2
screencapture -x "$OUT/shot-resume.png"
post /api/job/resume > /dev/null
check "I5 이어서 끝까지" wait_stage done 120
R=$(api /api/poll | pyget "str(d['job'].get('deletable'))+'/'+str(d['job'].get('failed'))+'/'+str(d['job'].get('livePairs'))")
check "I6 30개 모두 옮김 · 실패 0 · Live 짝 8 ($R)" test "$R" = "30/0/8"
sleep 1
screencapture -x "$OUT/shot-done.png"
F=$(ls "$DEST/2026/10/27" 2>/dev/null | wc -l | tr -d ' ')
check "I7 연/월/일 폴더 (2026/10/27 에 ${F}개, AAE 없음)" bash -c "[ '$F' = '11' ] && ! ls '$DEST/2026/10/27' | grep -q AAE"
check "I8 임시 폴더 정리됨" test ! -e "$DEST/.mpbroad-partial"
post /api/job/delete > /dev/null; sleep 3
LEFT=$(ls "$PHONE/DCIM/100APPLE" | grep -v AAE | wc -l | tr -d ' ')
check "I9 확인된 것만 iPhone에서 지움 (남은 파일 $LEFT)" test "$LEFT" = "0"
post /api/job/dismiss > /dev/null
find "$DEST" -type f | sed "s|$DEST/||" | sort > "$OUT/dest-tree.txt"
osascript -e "tell application \"$NAME\" to quit" >/dev/null 2>&1; wait_gone
rm -f "$SUP/test-fake-device"

echo "▶ E. 화면의 '종료' 버튼(= /api/quit) → 앱도 꺼짐"
launch
post /api/quit >/dev/null
check "E1 앱·엔진·도우미 모두 꺼짐" wait_gone

echo "▶ G. 업데이트 뒤 다시 열기 (/api/update/restart)"
launch; OLD=$(app_pid)
post /api/update/restart >/dev/null
sleep 3
check "G1 새로 열린 앱이 엔진을 켬" wait_ready
NEW=$(app_pid)
check "G2 앱이 새로 열림 ($OLD → ${NEW:-없음})" test -n "$NEW" -a "$NEW" != "$OLD"
check "G3 앱 하나만 떠 있음" test "$(pgrep -x "$NAME" | wc -l | tr -d ' ')" = "1"
osascript -e "tell application \"$NAME\" to quit" >/dev/null 2>&1
check "G4 끝나면 모두 꺼짐" wait_gone

echo "▶ 기록 모으기"
for f in launcher.log requests.log engine.log engine.prev.log engine.out engine.prev.out device.log settings.json imported.jsonl; do
  [ -f "$SUP/$f" ] && cp "$SUP/$f" "$OUT/$f"
done
sw_vers > "$OUT/macos.txt"
echo
echo "== 결과 =="; cat "$RES"
! grep -q '^FAIL' "$RES"
