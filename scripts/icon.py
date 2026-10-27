#!/usr/bin/env python3
"""app/AppIcon.svg → app/AppIcon.png (1024) · app/AppIcon.icns  (Playwright + Chromium, Pillow)
글자(Space Grotesk)는 앱 안의 글꼴 파일로 그려요."""
import asyncio, io, os
from playwright.async_api import async_playwright
from PIL import Image

ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
SVG = open(os.path.join(ROOT, "app", "AppIcon.svg")).read()
FONT = "file://" + os.path.join(ROOT, "engine", "web", "fonts", "SpaceGrotesk-Variable.woff2")
HTML = f"""<!doctype html><style>@font-face{{font-family:'Space Grotesk';font-weight:300 700;src:url('{FONT}') format('woff2')}}
html,body{{margin:0;background:transparent}}svg{{display:block}}</style>{SVG}"""

async def main():
    async with async_playwright() as p:
        b = await p.chromium.launch()
        pg = await b.new_page(viewport={"width": 1024, "height": 1024}, device_scale_factor=1)
        tmp = os.path.join(ROOT, "app", ".icon-render.html")
        open(tmp, "w").write(HTML)
        await pg.goto("file://" + tmp)
        os.remove(tmp)
        await pg.evaluate("document.fonts.load('700 10px \"Space Grotesk\"')")
        await pg.evaluate("document.fonts.ready")
        await pg.wait_for_timeout(300)
        png = await pg.screenshot(omit_background=True)
        await b.close()
    img = Image.open(io.BytesIO(png)).convert("RGBA")
    img.save(os.path.join(ROOT, "app", "AppIcon.png"))
    img.save(os.path.join(ROOT, "app", "AppIcon.icns"))
    img.resize((256, 256), Image.LANCZOS).save(os.path.join(ROOT, "engine", "web", "icon.png"))
    print("✔ app/AppIcon.png · app/AppIcon.icns")

asyncio.run(main())
