#!/usr/bin/env python3
"""Generate a macOS .icns app icon from a PNG, without any Apple tooling.

macOS normally builds .icns with `iconutil`/`sips` (Xcode Command Line Tools).
Those aren't installable on locked-down machines, so we build the .icns here
with Pillow instead and commit the result. The macOS build script then just
copies the committed assets/icon.icns into the app bundle — the Mac needs no
icon tooling at all.

Usage:
    python3 scripts/make-icns.py assets/logo.png            # -> assets/icon.icns
    python3 scripts/make-icns.py assets/logo.png out.icns

Requires: pip install pillow
"""
import sys
from pathlib import Path

try:
    from PIL import Image
except ImportError:
    sys.exit("Pillow is required: pip install pillow")

# The sizes Finder/Dock use, incl. @2x retina variants.
ICNS_SIZES = [16, 32, 64, 128, 256, 512, 1024]


def main() -> None:
    if len(sys.argv) < 2:
        sys.exit(__doc__)
    src = Path(sys.argv[1])
    dst = Path(sys.argv[2]) if len(sys.argv) > 2 else src.with_name("icon.icns")

    img = Image.open(src).convert("RGBA")

    # Pad to a square so the icon is never stretched.
    w, h = img.size
    if w != h:
        side = max(w, h)
        square = Image.new("RGBA", (side, side), (0, 0, 0, 0))
        square.paste(img, ((side - w) // 2, (side - h) // 2))
        img = square

    # Pillow's ICNS writer derives the smaller members from the base image.
    base = img.resize((1024, 1024), Image.LANCZOS)
    dst.parent.mkdir(parents=True, exist_ok=True)
    base.save(dst, format="ICNS", sizes=[(s, s) for s in ICNS_SIZES])
    print(f"wrote {dst} ({dst.stat().st_size} bytes)")


if __name__ == "__main__":
    main()
