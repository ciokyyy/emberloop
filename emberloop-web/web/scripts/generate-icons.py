#!/usr/bin/env python3
"""Generate PWA icons from web/public/logo.png (512x512).

Outputs (committed to web/public/):
  icons/icon-192.png, icons/icon-512.png       - direct resizes (purpose: any)
  icons/maskable-192.png, maskable-512.png     - logo on the dark UI background
                                                 with padding for the maskable safe zone
  apple-touch-icon.png                         - 180x180 on solid background (iOS, no transparency)

Run: python3 web/scripts/generate-icons.py
Requires: Pillow (pip install pillow).
"""
import sys
from pathlib import Path

try:
    from PIL import Image
except ImportError:
    sys.exit("Pillow is required: pip install pillow")

WEB = Path(__file__).resolve().parents[1]
SRC = WEB / "public" / "logo.png"
ICONS = WEB / "public" / "icons"
BG = (0x21, 0x21, 0x21, 255)  # --background #212121, the manifest theme/background color


def save(img, path, size):
    path.parent.mkdir(parents=True, exist_ok=True)
    img.save(path)
    print(f"{path.relative_to(WEB)} {size}x{size}")


def main():
    src = Image.open(SRC).convert("RGBA")
    save(src.resize((512, 512), Image.LANCZOS), ICONS / "icon-512.png", 512)
    save(src.resize((192, 192), Image.LANCZOS), ICONS / "icon-192.png", 192)
    for size in (512, 192):
        # Maskable: centered artwork at ~72% on the solid theme color so no
        # rounded-mask or circle-mask crop cuts the mark.
        canvas = Image.new("RGBA", (size, size), BG)
        art = src.resize((int(size * 0.72), int(size * 0.72)), Image.LANCZOS)
        canvas.alpha_composite(art, ((size - art.width) // 2, (size - art.height) // 2))
        bg = Image.new("RGB", (size, size), BG[:3])
        bg.paste(canvas, mask=canvas.split()[3])
        save(bg, ICONS / f"maskable-{size}.png", size)
    # Apple touch icon: 180x180, opaque background (iOS ignores transparency).
    canvas = Image.new("RGBA", (180, 180), BG)
    art = src.resize((int(180 * 0.78), int(180 * 0.78)), Image.LANCZOS)
    canvas.alpha_composite(art, ((180 - art.width) // 2, (180 - art.height) // 2))
    bg = Image.new("RGB", (180, 180), BG[:3])
    bg.paste(canvas, mask=canvas.split()[3])
    save(bg, WEB / "public" / "apple-touch-icon.png", 180)


if __name__ == "__main__":
    main()
