#!/usr/bin/env python3
"""시험용 가짜 iPhone 사진 만들기 → scripts/testdata/DCIM/100APPLE + scripts/testdata/dates.txt
(풍경 그림 JPG · Live Photo 짝(JPG+MOV) · 동영상(MOV, 내용은 아무 바이트) · 편집 정보 AAE)
git 은 파일 날짜를 기억하지 않아서, 찍은 날짜는 dates.txt 에 적고 scripts/fake-iphone.sh 가 맞춰 줘요."""
import os, random
from PIL import Image, ImageDraw

ROOT = os.path.join(os.path.dirname(os.path.abspath(__file__)), "testdata")
DCIM = os.path.join(ROOT, "DCIM", "100APPLE")
os.makedirs(DCIM, exist_ok=True)
for f in os.listdir(DCIM):
    os.remove(os.path.join(DCIM, f))
random.seed(7)
SCENES = [
    ("#A9C9E3", "#FFF2C4", "#5D8A6A"), ("#F3C9A8", "#FFF6E5", "#8C5A4A"), ("#2F3E5C", "#E9E3C9", "#18202F"),
    ("#CFE3D7", "#FFFFFF", "#3F7D6E"), ("#E8D9C4", "#C77B4A", "#B89A74"), ("#9DB8C9", "#F5F0DC", "#4C6A80"),
    ("#F0B9B0", "#FFE9C7", "#6E4E6A"), ("#D9DEE4", "#FFFFFF", "#7A8A72"),
]

def scene(i, w=640, h=480):
    sky, sun, land = SCENES[i % len(SCENES)]
    im = Image.new("RGB", (w, h), sky)
    d = ImageDraw.Draw(im)
    for y in range(h // 2):  # 하늘 위쪽을 살짝 어둡게
        a = int(18 * (1 - y / (h / 2)))
        d.line([(0, y), (w, y)], fill=tuple(max(0, c - a) for c in im.getpixel((0, y))))
    sx, sy = random.randint(80, w - 80), random.randint(50, 150)
    d.ellipse([sx - 40, sy - 40, sx + 40, sy + 40], fill=sun)
    pts = [(0, h)]
    x = 0
    while x <= w:
        pts.append((x, random.randint(int(h * .5), int(h * .75))))
        x += random.randint(60, 140)
    pts += [(w, random.randint(int(h * .5), int(h * .75))), (w, h)]
    d.polygon(pts, fill=land)
    near = tuple(max(0, int(int(land[k:k + 2], 16) * .7)) for k in (1, 3, 5))
    d.polygon([(0, h), (0, int(h * .85)), (w * .4, int(h * .8)), (w, int(h * .9)), (w, h)], fill=near)
    return im

# (날짜, 장 수)
days = [("202610271412", 9), ("202610261830", 6), ("202610190915", 7), ("202609051120", 4), ("202512240800", 4)]
lines = []
n = 4821
i = 0
for stamp, count in days:
    for k in range(count):
        name = f"IMG_{n}"
        t = stamp[:-2] + f"{(int(stamp[-2:]) + k) % 60:02d}"
        kind = "live" if k % 4 == 1 else "video" if k % 5 == 3 else "photo"
        if kind == "video":
            with open(os.path.join(DCIM, name + ".MOV"), "wb") as f:
                f.write(os.urandom(random.randint(120_000, 260_000)))
            lines.append(f"{t} {name}.MOV")
        else:
            scene(i).save(os.path.join(DCIM, name + ".JPG"), quality=78)
            lines.append(f"{t} {name}.JPG")
            if kind == "live":
                with open(os.path.join(DCIM, name + ".MOV"), "wb") as f:
                    f.write(os.urandom(random.randint(40_000, 90_000)))
                lines.append(f"{t} {name}.MOV")
                with open(os.path.join(DCIM, name + ".AAE"), "w") as f:
                    f.write("<plist/>")
                lines.append(f"{t} {name}.AAE")
        n += 1
        i += 1
with open(os.path.join(ROOT, "dates.txt"), "w") as f:
    f.write("\n".join(lines) + "\n")
total = sum(os.path.getsize(os.path.join(DCIM, x)) for x in os.listdir(DCIM))
print(f"✔ {len(os.listdir(DCIM))} files, {total // 1024} KB")
