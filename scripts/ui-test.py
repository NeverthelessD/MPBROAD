#!/usr/bin/env python3
"""리눅스에서 화면 · 가져오기 흐름 시험 (가짜 iPhone, Playwright + Chromium)
   python3 scripts/ui-test.py [결과 폴더]   → 스크린샷 · results.txt
   엔진은 engine/ 에서 `go build -o /tmp/mpb-engine .` 로 미리 만들어 두세요."""
import asyncio, json, os, shutil, signal, subprocess, sys, time, urllib.request
from playwright.async_api import async_playwright

ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
OUT = sys.argv[1] if len(sys.argv) > 1 else "/tmp/mpb-ui"
T = "/tmp/mpb-uitest"
PORT, TOK = 47998, "tok"
ENGINE = "/tmp/mpb-engine"
shutil.rmtree(OUT, ignore_errors=True); os.makedirs(OUT)
shutil.rmtree(T, ignore_errors=True); os.makedirs(T)
PHONE = f"{T}/iphone"
subprocess.run(["bash", f"{ROOT}/scripts/fake-iphone.sh", PHONE], check=True, stdout=subprocess.DEVNULL)
results = []
proc = None

def ok(name, cond, extra=""):
    results.append(("PASS" if cond else "FAIL") + "  " + name + (f"  ({extra})" if extra else ""))
    print(results[-1])

def start_engine():
    global proc
    env = dict(os.environ, MPB_SUPPORT=f"{T}/sup", MPB_FAKE_DEVICE=PHONE, MPB_DEFAULT_DEST=f"{T}/dest", MPB_TOKEN=TOK, MPB_PORT=str(PORT))
    proc = subprocess.Popen([ENGINE], env=env, stdout=open(f"{T}/engine.out", "a"), stderr=subprocess.STDOUT)
    for _ in range(50):
        try:
            api("/api/ping"); return
        except Exception:
            time.sleep(0.1)

def api(path, body=None):
    req = urllib.request.Request(f"http://127.0.0.1:{PORT}{path}", headers={"x-token": TOK, "content-type": "application/json"},
                                 data=json.dumps(body).encode() if body is not None else None)
    return json.load(urllib.request.urlopen(req, timeout=5))

def rate(n):
    p = f"{PHONE}/.rate"
    if n: open(p, "w").write(str(n))
    elif os.path.exists(p): os.remove(p)

async def until(pg, js, timeout=15000):
    await pg.wait_for_function(js, timeout=timeout)

async def main():
    start_engine()
    async with async_playwright() as p:
        b = await p.chromium.launch()
        ctx = await b.new_context(viewport={"width": 1280, "height": 820}, color_scheme="light")
        pg = await ctx.new_page()
        errs = []
        pg.on("pageerror", lambda e: errs.append(str(e)))
        pg.on("dialog", lambda d: asyncio.ensure_future(d.accept()))
        await pg.goto(f"http://127.0.0.1:{PORT}/?t={TOK}")
        await pg.wait_for_selector(".tile")
        await pg.wait_for_timeout(1200)
        await pg.screenshot(path=f"{OUT}/01-main.png")
        n = await pg.locator(".tile").count()
        ok("메인: 사진 30개(Live 묶음 포함) 보임", n == 30, f"{n}개")
        ok("Live Photo 표시 8개", await pg.locator(".tile .live").count() == 8)
        ok("연/월 이동 막대 (연 2 · 월 3)", await pg.locator("#rail button[data-y]").count() == 2 and await pg.locator("#rail button[data-m]").count() == 3)
        # 연도 통째로 고르기
        await pg.click(".ysel[data-year='2026']")
        txt = await pg.locator(".actbar .info b").inner_text()
        ok("2026년 모두 선택 → 26개", txt.startswith("26개 선택"), txt)
        ok("연도 버튼이 '선택 풀기'로 바뀜", "선택 풀기" in await pg.locator(".ysel[data-year='2026']").inner_text())
        await pg.click("#rail button[data-y='2025']")
        await pg.wait_for_timeout(900)
        await pg.screenshot(path=f"{OUT}/01b-year.png")
        await pg.click(".ysel[data-year='2025']")
        txt = await pg.locator(".actbar .info b").inner_text()
        ok("2025년도 더해서 30개", txt.startswith("30개 선택"), txt)
        await pg.click(".ysel[data-year='2026']"); await pg.click(".ysel[data-year='2025']")
        ok("다시 누르면 모두 풀림", "새 항목 30개" in await pg.locator(".actbar .info b").inner_text())
        await pg.click(".month button[data-month='2026-10']")
        ok("이 달 모두 (10월 22개)", (await pg.locator(".actbar .info b").inner_text()).startswith("22개 선택"))
        await pg.click(".month button[data-month='2026-10']")
        await pg.locator("#scroller").evaluate("e => e.scrollTop = 0")

        tiles = pg.locator(".tile")
        await tiles.nth(1).click(); await tiles.nth(2).click()
        await tiles.nth(5).click(modifiers=["Shift"])
        txt = await pg.locator(".actbar .info b").inner_text()
        ok("고르기 + Shift 범위 고르기", txt.startswith("5개 선택"), txt)
        await pg.screenshot(path=f"{OUT}/02-selected.png")
        await pg.click("#previewBtn")
        await pg.wait_for_selector(".list")
        ok("미리보기 끔 → 목록", await pg.locator(".list .row:not(.head)").count() == 30)
        await pg.screenshot(path=f"{OUT}/03-list.png")
        await pg.click("#previewBtn")
        await pg.keyboard.press("Escape")

        # ① 가져오기 → ② 잠김 → 자동 이어서 → ④ 끝 → 지우기
        rate(120_000)
        await pg.click(".sec >> nth=0 >> button[data-day]")
        await pg.click(".actbar .go")
        await pg.wait_for_selector(".scrim .bigpct")
        await pg.wait_for_timeout(1500)
        await pg.screenshot(path=f"{OUT}/04-importing.png")
        ok("① 가져오는 중 창", "가져오는 중" in await pg.locator(".scrim h2").inner_text())
        open(f"{PHONE}/.locked", "w").close()
        await until(pg, "document.querySelector('.scrim h2') && document.querySelector('.scrim h2').textContent.includes('잠겨서')")
        await pg.wait_for_timeout(800)
        await pg.screenshot(path=f"{OUT}/05-locked.png")
        ok("② 잠김 → 잠시 멈춤 창", True)
        os.remove(f"{PHONE}/.locked")
        rate(0)
        await until(pg, "document.querySelector('.scrim h2') && document.querySelector('.scrim h2').textContent.includes('안전하게')", 30000)
        await pg.screenshot(path=f"{OUT}/06-done.png")
        st = api("/api/poll")["job"]
        ok("④ 끝: 9개 옮김 · Live 짝 · 지울지 묻기", st.get("deletable") == 9 and st.get("askDelete") is True and st.get("livePairs") == 2, f"{st.get('deletable')} {st.get('livePairs')}")
        day = [f for f in os.listdir(f"{T}/dest/2026/10/27")]
        ok("연/월/일 폴더 정리 (2026/10/27)", len(day) == 11 and not any(x.endswith(".AAE") for x in day), f"{len(day)}개")
        await pg.click(".scrim .bd")
        await until(pg, "document.querySelector('.scrim h2') && document.querySelector('.scrim h2').textContent.includes('지웠어요')")
        await pg.screenshot(path=f"{OUT}/07-deleted.png")
        left = [f for f in os.listdir(f"{PHONE}/DCIM/100APPLE") if f.startswith("IMG_482") and not f.endswith(".AAE")]
        ok("iPhone에서 확인된 것만 지움", api("/api/poll")["job"].get("deleted") == 9)
        await pg.click(".scrim .b1")
        await pg.wait_for_timeout(1500)
        ok("지운 뒤 목록 21개", await pg.locator(".tile").count() == 21)

        # ⑤ 고른 사진 지우기 (먼저 옮긴 뒤)
        await pg.locator(".tile").nth(0).click(); await pg.locator(".tile").nth(1).click(); await pg.locator(".tile").nth(2).click()
        await pg.click(".actbar .del")
        await pg.wait_for_selector(".choice")
        await pg.screenshot(path=f"{OUT}/08-delete-plan.png")
        ok("⑤ 지우기 창: 먼저 옮긴 뒤 지우기가 기본", await pg.locator(".choice.on input[value='import-first']").count() == 1)
        await pg.click(".scrim .bd")
        await until(pg, "document.querySelector('.scrim h2') && document.querySelector('.scrim h2').textContent.includes('지웠어요')", 20000)
        ok("⑤ 옮긴 뒤 3개 지움", api("/api/poll")["job"].get("deleted") == 3)
        await pg.click(".scrim .b1")

        # ③ 앱이 갑자기 꺼짐 → 다시 켜면 이어서 하기
        await pg.wait_for_timeout(800)
        rate(150_000)
        await pg.click(".actbar .go")
        await pg.wait_for_selector(".scrim .bigpct")
        await pg.wait_for_timeout(2500)
        proc.send_signal(signal.SIGKILL); proc.wait()
        await pg.wait_for_timeout(1500)
        await pg.screenshot(path=f"{OUT}/09-lost.png")
        rate(0)
        start_engine()
        await until(pg, "document.querySelector('.scrim h2') && document.querySelector('.scrim h2').textContent.includes('끝나지 않았어요')", 15000)
        await pg.screenshot(path=f"{OUT}/10-resume.png")
        ok("③ 지난번 가져오기 이어서 하기 창", True)
        await pg.click(".scrim .b1")
        await until(pg, "document.querySelector('.scrim h2') && (document.querySelector('.scrim h2').textContent.includes('안전하게') || document.querySelector('.scrim h2').textContent.includes('옮겼어요'))", 30000)
        st = api("/api/poll")["job"]
        ok("③ 이어서 끝까지", st.get("failed") == 0, json.dumps({k: st.get(k) for k in ("ok", "skipped", "failed", "deletable")}))
        await pg.click(".scrim .b2")  # 지우지 않기
        await pg.wait_for_timeout(1200)
        ok("모두 옮긴 뒤 새 항목 0", (await pg.locator("#tabs button[data-tab=new]").inner_text()).endswith(" 0"))
        await pg.click("#tabs button[data-tab=done]")
        await pg.wait_for_timeout(500)
        await pg.screenshot(path=f"{OUT}/11-done-tab.png")

        # 설정
        await pg.click("#settingsBtn")
        await pg.wait_for_selector(".sheet .sbox")
        await pg.screenshot(path=f"{OUT}/12-settings.png", full_page=True)
        await pg.click("[data-ap=dark]")
        await pg.wait_for_timeout(300)
        await pg.screenshot(path=f"{OUT}/13-settings-dark.png")
        await pg.click("[data-ap=system]")
        await pg.click(".sheet [data-act=close]")

        # 연결 화면
        os.rename(PHONE, PHONE + ".off")
        await pg.wait_for_selector(".connect", timeout=8000)
        await pg.wait_for_timeout(500)
        await pg.screenshot(path=f"{OUT}/14-connect.png")
        ok("케이블 빠짐 → 연결 화면", True)
        os.rename(PHONE + ".off", PHONE)
        open(f"{PHONE}/.locked", "w").close()
        await until(pg, "document.querySelector('.connect h1') && document.querySelector('.connect h1').textContent.includes('잠금')")
        await pg.screenshot(path=f"{OUT}/15-connect-locked.png")
        os.remove(f"{PHONE}/.locked")
        await pg.wait_for_selector(".tile", timeout=8000)

        await pg.set_viewport_size({"width": 760, "height": 820})
        await pg.wait_for_timeout(600)
        await pg.screenshot(path=f"{OUT}/16-narrow.png")
        ok("화면 오류 없음", not errs, "; ".join(errs[:3]))
        await b.close()
    proc.terminate()
    open(f"{OUT}/results.txt", "w").write("\n".join(results) + "\n")
    print("\n".join(results))
    sys.exit(1 if any(r.startswith("FAIL") for r in results) else 0)

try:
    asyncio.run(main())
finally:
    if proc and proc.poll() is None:
        proc.terminate()
