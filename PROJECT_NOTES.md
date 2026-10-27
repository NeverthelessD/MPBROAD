> **이 파일을 읽은 Claude는 작업이 끝날 때마다 이 파일을 업데이트할 것.**
> (구조가 바뀌면 「구조」, 새로 정한 것은 「결정 사항」, 한 일·남은 일은 「진행 사항」에 적고 커밋·푸시까지 할 것)

# MyPhotoBook_ROAD — 프로젝트 노트

iPhone 사진 · 동영상을 케이블로 맥(Apple Silicon)에 옮기는 앱. 연/월/일 폴더 정리 · 이미 옮긴 것 건너뛰기 · Live Photo 묶음 ·
복사 확인 뒤에만 iPhone에서 지우기 · 멈춘 곳부터 이어서 하기. made by. Nevertheless_D
사용자는 개발자가 아니에요. 한국어로 쉽게 설명하고, 터미널 사용을 요구하지 말 것. 결과물은 더블클릭으로 쓸 수 있게.

---

## 진행 사항

작업이 중간에 끊기면 아래 체크리스트에서 **처음으로 체크 안 된 항목부터** 이어서 하면 돼요. 단계마다 커밋 · 푸시.

- [x] 0. 디자인 시안 → 사용자 확정 (2026-10-27, Design 캔버스 'MyPhotoBook_ROAD 디자인 시안', 방향 B 라이트박스)
- [x] 1. 저장소 뼈대 (이 파일, CLAUDE.md, VERSION, .gitignore, 글꼴)
- [x] 2. 엔진 기본: 서버 · 토큰 · 지킴이 · 업데이트 · 진단 (MSROAD 1.3 방식)
- [x] 3. iPhone 연결 도우미(Swift, ImageCaptureCore) + 가짜 기기(MPB_FAKE_DEVICE)
- [x] 4. 가져오기: 복사 → 확인 → 정리, 중복 건너뛰기, Live Photo 묶음, 이어서 하기, 잠김 멈춤, 지우기
- [x] 5. 화면 (시안 B 그대로)
- [x] 6. 맥 앱 껍데기 · 아이콘 · build-mac.sh · smoke-mac.sh · ci-mac.sh · 워크플로
- [x] 7. 리눅스 Playwright 시험 (가짜 iPhone)
- [x] 8. 맥 빌드 서버 통과 · 사용 가이드 이미지 · 설치 zip · v1.0.0 릴리스

### 다음에 할 수 있는 일
- [ ] 사용자 맥 + 실제 iPhone 으로 확인: 목록 · 썸네일 화질 · Live Photo 묶음(groupUUID/이름) · 동영상 길이 · 잠김 감지 ·
      지우기(requestDeleteFiles) · 큰 동영상 이어 받기. 빌드 서버에는 iPhone 이 없어서 이 부분은 가짜 iPhone 으로만 시험했어요.
- [ ] 문제가 나면 사용자에게 '진단 정보 보기'(device.log 포함) 내용을 받아서 고치기

### 작업 기록
- 2026-10-27 v1.1.0 — 사용자 요청: 옮기기 전 · 뒤에 사진을 크게 확인 (지울지 판단용). 시안 → 사용자 결정: 길게 누르면 팝업,
  **손을 떼면 바로 닫힘**, 전체 화면 보기는 없음 (캔버스 Peek · PeekHold, 확정).
  화면: 타일/목록 줄을 0.4초 누르면 동그라미가 차오르고 팝업(#peek, pointer-events 없음) · 손 떼면 닫고 그 누르기는 고르기로 안 침
  (suppressClick) · 마우스를 올리고 Space 를 누르는 동안. Live 는 영상 자동 재생, 동영상은 소리 없이.
  엔진 view.go: /api/view?id=<묶음>&p=photo|motion · /api/view/status. 옮긴 것은 기록의 맥 파일을 그대로(다시 안 받음),
  아직 iPhone 에만 있으면 Support/cache/view 에 받아 둠(이어 받기 · 이틀/3GB 넘으면 정리). HEIC 등은 sips 로 JPEG(2560px).
- 2026-10-27 v1.0.3 — 사용자 질문: 잠시 멈춤 → 케이블 빼기 → 다시 연결 → 이어서 하기 되나? → 됨(시험 추가: TestPauseUnplugReplugResume).
  보강: 다시 연결했을 때 iPhone 이 찍은 날짜를 다르게 알려 주면 파일 id 가 바뀌어 '찾을 수 없음'이 되던 경우 →
  Library.FileLoose(같은 이름 · 같은 크기가 하나뿐이면 그것)로 이어서 함 (TestResumeWhenDateReportedDifferently).
- 2026-10-27 v1.0.2 — 사용자 요청: 옮긴 뒤 맥에서 실수로 지운 사진을 다시 옮길 수 있게. '이미 옮김'만 고르면 버튼이 꺼져 있던 문제.
  엔진: counts.gone(기록은 있는데 맥에 없음), /api/import 의 redo(기록을 보지 않고 다시 옮기기 — 정리될 자리에 같은 크기 파일이 있으면
  사본 없이 건너뜀), 결과에 freeBytes. 화면: 타일 '맥에 없음'(주황), 알림 'n개가 맥에서 없어졌어요 · 골라서 다시 옮기기',
  이미 옮긴 항목이 섞이면 '다시 옮기기' 창(새 항목만 / 없어진 것도 다시(추천) / 모두 다시). 시험: go test + ui-test 29개.
- 2026-10-27 v1.0.1 — 사용자 요청: 연도별로 통째로 골라서 한 번에 옮기기. 목록에 연도 머리(큰 연도 숫자 · 새 항목 수 · 크기 ·
  'YYYY년 모두 선택' ↔ '선택 풀기', 일부만 골랐으면 (n/전체)), 달 머리에 '이 달 모두', 오른쪽 막대의 연도도 눌러서 그 해로 이동.
  app.js: toggleGroup · syncGroupButtons · pickable. ui-test.py 에 연도 선택 시험 추가(23개 통과).
- 2026-10-27 v1.0.0 제작: 엔진(Go) · iPhone 도우미(Swift, ImageCaptureCore) · 화면 · 껍데기 · 아이콘 · 가이드 · 빌드 서버.
  리눅스: go test(가져오기 · 정리 · Live 이름 · 잠김/케이블 빠짐 → 이어 받기 · 앱 꺼짐 → 이어서 · 지우기) + scripts/ui-test.py(18개 통과).
  맥 빌드 서버 1차: iPhone 도우미의 requestContentsAuthorization/requestControlAuthorization 은 macOS 에서 쓸 수 없음(iOS 전용) → 뺌.
  맥 빌드 서버 2차: build=ok · smoke=ok (25/25, macOS 15.7). 스크린샷이 확정 시안과 같음 확인. v1.0.0 릴리스 · 처음 설치용 zip 전달.
- 2026-10-27 디자인 확정. 사용자 요청 정리:
  날짜별(연/월/일) 자동 정리, 저장 폴더는 기본(사진/MyPhotoBook) + 다른 폴더 지정, 사진 · 동영상 같은 방식,
  이미 옮긴 것 건너뛰기, 복사 확인 후에만 iPhone에서 지우기, Live Photo 묶음 유지, 미리보기 켜고 끄기(기본 켬),
  한 번에 옮기는 개수 제한 없음, iPhone 화면이 꺼져도 진행(→ 막을 수 없어서 '잠김 → 잠시 멈춤 → 잠금 풀면 자동 이어서' +
  자동 잠금 '안 함' 안내), 문제로 멈추면 다음에 멈춘 곳부터, 사진을 골라서 iPhone에서 지우기(먼저 옮긴 뒤 지우기가 기본).

## 결정 사항

- 이름: 앱 `MyPhotoBook_ROAD.app` (실행 파일 MyPhotoBook_ROAD), 엔진 `mpbroad-engine`, iPhone 도우미 `mpbroad-device`,
  번들 id `com.nevertheless-d.myphotobook-road`, 데이터 폴더 `~/Library/Application Support/MyPhotoBook_ROAD`, 환경 변수 접두사 `MPB_`,
  주소 127.0.0.1:47931 (MSROAD 47917 과 겹치지 않게).
- 업데이트: `releases/latest.json` (raw.githubusercontent.com/NeverthelessD/MPBROAD/main/releases/), 켤 때 + 6시간마다, 자동 설치 기본 켬.
  화면 · 문구 · 가이드에 GitHub 나 저장소 이름은 쓰지 않음 ("새 버전이 나오면 앱이 알아서 받아요").
- 화면 아래(왼쪽) `made by. Nevertheless_D`.

### 확정 디자인 (방향 B · 라이트박스)
- 색 (라이트 / 다크)
  - 바탕 #EEF1F5 / #0C0F14, 판 #FFFFFF / #151A22, 칩 #E2E7EE / #1D2430, 선 #D3DAE3 / #2A3342
  - 글자 #0F1720 / #E8EDF4, 흐린 글자 #556170 / #9AA6B5
  - 강조(코발트) #2D5BFF / #4F6FFF, 강조 옅은 바탕 #E3E9FF / #1C2550, 강조 글자 #2448D8 / #9DB0FF
  - LIVE · 주의(주황) #FF8A55, 주의 글자 #A8410F / #FFAE85, 지우기(빨강) #C4261B / #D93B30
  - 아래 떠 있는 실행 막대: 라이트에선 짙은 #0F1720 + 흰 글자, 다크에선 밝은 #E8EDF4 + 짙은 글자
- 글꼴(앱 안에 들어 있음, OFL): 본문 IBM Plex Sans KR(400/500/600), 로고 · 숫자 Space Grotesk(가변)
- 메인: 위 막대(신호등 자리 · 로고 · iPhone 칩 · 미리보기 켬/끔 · 설정) → 걸러 보기(새 항목/이미 옮김/전체 + 모두/사진/동영상/Live Photo)
  → 날짜 묶음(왼쪽 큰 날짜 숫자 · 요일 · 새 항목 수 · '이 날 모두') + 정사각 격자(모서리 4px, 간격 4px) → 오른쪽 연/월 이동 막대
  → 아래 떠 있는 실행 막대(선택 수 · 크기 · 저장 위치 · 폴더 변경 · 확인 후 iPhone에서 지우기 · 선택 항목 지우기 · 가져오기)
- 미리보기 끔: 같은 날짜 묶음, 격자 대신 목록(이름 · 종류 · 크기 · 상태) + "미리보기를 끄면…" 안내
- 가져오기 창(가운데 카드): ① 진행 중(큰 % · 01 복사 · 02 확인 · 03 지우기 대기 · 지금 파일) ② iPhone 잠김 · 잠시 멈춤
  ③ 지난번 가져오기가 끝나지 않았어요 · 이어서 하기 ④ 다 옮김 · iPhone에서 지울까요 ⑤ 고른 사진 지우기(먼저 옮긴 뒤 지우기 기본)
- 연결 화면: 그림 + 01 케이블 · 02 잠금 풀기 · 03 신뢰 + 자동 잠금 끄기 팁
- 설정: 저장 위치 · 폴더 정리 예시 · 가져오기 스위치 6개 · 화면(맥 설정 따름/라이트/다크) · 업데이트(버전 · 업데이트 확인 · 진단 정보 보기)
- 아이콘: 초록 천 표지(#1F6B52, 왼쪽 등) 위에 겹친 사진 세 장 + 아래에 MyPhotoBook(#FBF3DF) · _ROAD(#F2B23A) 두 줄

## 구조

```
app/shell/main.swift    MyPhotoBook_ROAD.app 본체(Swift/AppKit): Dock 아이콘 · 메뉴 · WKWebView 창 · 엔진 실행/종료 · Dock 다시 열기 ·
                        업데이트 후 다시 열기(restart-requested → NSWorkspace) · 가져오는 동안 잠자기 막기(beginActivity) ·
                        가져오는 중 ⌘Q 하면 한 번 묻기(MPB_NO_QUIT_ASK=1 이면 안 물음) · launcher.log
app/device/main.swift   mpbroad-device: ImageCaptureCore 로 iPhone 찾기 · 목록 · 썸네일 · 이어 받기(requestReadData 4MB씩 + sha256) ·
                        지우기(requestDeleteFiles, didRemove 로 확인) · 잠김 감지(isAccessRestrictedAppleDevice). 엔진의 자식, stdin 이 닫히면 끝.
app/Info.plist          LSUIElement 없음, NSAllowsLocalNetworking, 자동/갑작스런 종료 끔, NSCamera/PhotoLibraryUsageDescription
app/AppIcon.svg         → scripts/icon.py → AppIcon.png · AppIcon.icns (Space Grotesk 글자)
engine/main.go          서버 · 토큰 · 설정(settings.json) · 저장 위치(기본 ~/Pictures/MyPhotoBook, 다른 폴더) · 라우트
engine/device.go        Device 인터페이스 + Swift 도우미와 JSON 줄 주고받기(다시 켜기 · 응답 기다리기 · 2분 무응답이면 실패)
engine/fakedevice.go    시험용 가짜 iPhone (폴더, .locked = 잠김, 폴더 없음 = 빠짐, .rate = 천천히)
engine/library.go       파일 → 묶음(Asset): Live Photo(같은 폴더 · 같은 이름 사진+MOV) · RAW+JPEG · AAE 등은 숨김.
                        파일 id = sha1(폴더|이름|크기|찍은시각)[:16]. 이미 옮김 = 기록(imported.jsonl) 또는 정리될 자리에 같은 크기 파일
engine/journal.go       Support/imported.jsonl (가져온 기록 · 지운 표시)
engine/jobs.go          가져오기 작업: job.json(계획) + job.log(파일마다 결과) → 앱이 꺼져도 이어서. 받는 중 파일은
                        <저장 폴더>/.mpbroad-partial/<id>.part (같은 디스크라 rename 으로 정리). 잠김/빠짐 → 기다렸다 자동 이어서.
                        확인 = 크기 + 다시 읽은 sha256 == 받으며 계산한 sha256. 같은 이름 다른 파일이면 _1 (Live 는 짝도 같이).
                        지우기: 확인된 것만(onlyOnMac) · 바로 지우기(direct) · 먼저 옮긴 뒤 지우기(autoDelete)
engine/thumbs.go        Support/cache/thumbs/<id>.jpg (320px, 60일 안 보면 지움)
engine/update.go        releases/latest.json 자동 업데이트 (MSROAD 와 같음, 맥 전용)
engine/supervise.go     지킴이 (MSROAD 와 같음)  · diag.go: /diag 진단 페이지(device.log 포함)
engine/web/             index.html · css/app.css(확정 디자인 토큰) · js/app.js(화면 전체) · fonts(IBM Plex Sans KR, Space Grotesk, OFL)
scripts/build-mac.sh    (맥) go test → Go 엔진 + swiftc 껍데기 · 도우미 → 서명 → dist/MPBROAD-<v>.zip · dist/MyPhotoBook_ROAD.zip
scripts/smoke-mac.sh    (맥) A 켜기·진짜 도우미·자식 관계 B 다시 열기 C 도우미/엔진 고장 → 다시 켬 · ⌘Q
                        I 가짜 iPhone 가져오기 → 강제 종료 → 이어서 → 정리 확인 → 지우기  E 종료 버튼  G 업데이트 뒤 다시 열기
scripts/ci-mac.sh       빌드 서버 진입점 → ci-results 브랜치 (status.json · results.txt · shot-*.png · zip · 기록)
scripts/release.sh      ci-results 확인 → releases/MPBROAD-<v>.zip · latest.json · dist/MyPhotoBook_ROAD.zip
scripts/ui-test.py      리눅스 화면 시험 (Playwright, 가짜 iPhone) · scripts/make-testdata.py · fake-iphone.sh · testdata/
scripts/guide.py        docs/guide.html → docs/MyPhotoBook_ROAD_사용가이드.png
```
엔진 실행 사슬: MyPhotoBook_ROAD.app → mpbroad-engine(지킴이, MPB_SUPERVISOR=1) → mpbroad-engine(엔진, MPB_CHILD=1) → mpbroad-device.

### 리눅스에서 시험하는 법
`cd engine && go test ./... && go build -o /tmp/mpb-engine .` → `python3 scripts/ui-test.py /tmp/mpb-ui` (스크린샷 · results.txt).
손으로: `bash scripts/fake-iphone.sh /tmp/x/iphone` 후 `MPB_SUPPORT=/tmp/x/sup MPB_FAKE_DEVICE=/tmp/x/iphone MPB_DEFAULT_DEST=/tmp/x/dest
MPB_TOKEN=tok MPB_PORT=47999 /tmp/mpb-engine` → http://127.0.0.1:47999/?t=tok

## 업데이트 내보내는 법 (Claude용)
1. 수정 → `VERSION` 올리기 → 커밋 · 푸시 (app/ · engine/ · VERSION 이 바뀌면 맥 빌드 서버가 자동으로 돎, 5~10분)
2. `git ls-remote origin ci-results` 가 바뀔 때까지 기다린 뒤:
   `git fetch origin +refs/heads/ci-results:refs/remotes/origin/ci-results && git archive origin/ci-results | tar -x -C <임시폴더>`
   → status.json(build/smoke 모두 ok) · results.txt · shot-*.png 확인
3. `scripts/release.sh --notes "바뀐 점 1" "바뀐 점 2"` → 커밋 · 푸시. 처음 설치용은 dist/MyPhotoBook_ROAD.zip.
