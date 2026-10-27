#!/usr/bin/env python3
"""docs/guide.html → docs/MyPhotoBook_ROAD_사용가이드.png  (Playwright + Chromium)"""
import asyncio, os
from playwright.async_api import async_playwright
ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))

async def main():
    async with async_playwright() as p:
        b = await p.chromium.launch()
        pg = await b.new_page(viewport={"width": 1200, "height": 800}, device_scale_factor=2)
        await pg.goto("file://" + os.path.join(ROOT, "docs", "guide.html"))
        await pg.evaluate("document.fonts.ready")
        await pg.wait_for_timeout(800)
        out = os.path.join(ROOT, "docs", "MyPhotoBook_ROAD_사용가이드.png")
        await pg.screenshot(path=out, full_page=True)
        await b.close()
    print("✔", out)

asyncio.run(main())
