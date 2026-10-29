# /// script
# requires-python = ">=3.10"
# dependencies = ["fonttools==4.60.1", "brotli==1.1.0"]
# ///
"""Subset Geist and Geist Mono into the woff2 files in src/fonts.

Run by hand from site/: `uv run scripts/subset-fonts.py`, then paste the
printed unicode-range values into src/styles/fonts.css.

`--limit-weights` also cuts the wght axis to 400-700. It saves about 8.6 KB
on the two core files at the time of writing but shifts glyph edges by sub-pixel amounts, so it is
off by default. With it, set font-weight: 400 700 in fonts.css.

Rerun when the page starts using a character outside src/fonts (anything
other than CORE and Latin Extended-A), a new font-feature-settings value,
or a new Geist release. With --limit-weights, also rerun for a weight
outside 400-700.
"""

import hashlib
import io
import sys
import urllib.request
from pathlib import Path

from fontTools import subset
from fontTools.ttLib import TTFont
from fontTools.varLib import instancer

# google/fonts commits holding Geist 1.800 and Geist Mono 1.701, the same
# outlines @fontsource-variable 5.3.0 shipped.
SOURCES = {
    "geist": (
        "https://raw.githubusercontent.com/google/fonts/87a44ad3d4535f7b3a5a715a08e8944f144afd06/ofl/geist/Geist%5Bwght%5D.ttf",
        "73894e0448cae90a92b6c2f8732b7bb9acb7b94c418bff559dad4a18e1de9659",
    ),
    "geist-mono": (
        "https://raw.githubusercontent.com/google/fonts/9e25e2ba265e5298f70f6182dd4e8a3ebf1b9123/ofl/geistmono/GeistMono%5Bwght%5D.ttf",
        "d00e590b8eb3a59acc329b2d044fd143ae935090b7da33199ebee27cc7de8196",
    ),
}

# global.css uses 400, 500, 600, 620, 650 and 700 (bold <b>/<strong>).
WEIGHT_RANGE = (400, 700)
LIMIT_WEIGHTS = "--limit-weights" in sys.argv[1:]

# Default-on features only; the page sets no font-feature-settings.
LAYOUT_FEATURES = ["ccmp", "locl", "liga", "kern", "mark", "mkmk"]

# Core matches the fontsource "latin" file's unicode-range. Browsers shape
# each file separately, so a split between two characters drops the kerning
# between them; keeping the old boundary keeps text rendering identical.
# Splitting Latin-1 out of core would unkern pairs such as "Ég".
CORE = (
    set(range(0x20, 0x7F))
    | set(range(0xA0, 0x100))
    | {0x0131, 0x0152, 0x0153, 0x02BB, 0x02BC, 0x02C6, 0x02DA, 0x02DC}
    | {0x0304, 0x0308, 0x0329, 0x20AC, 0x2122, 0x2191, 0x2193, 0x2212}
    | {0x2215, 0xFEFF, 0xFFFD}
    | set(range(0x2000, 0x2070))
)
EXTENDED = set(range(0x100, 0x180)) - CORE

OUT_DIR = Path(__file__).resolve().parent.parent / "src" / "fonts"


def load_source(url: str, sha256: str) -> TTFont:
    data = urllib.request.urlopen(url).read()
    if hashlib.sha256(data).hexdigest() != sha256:
        raise SystemExit(f"checksum mismatch for {url}")
    return TTFont(io.BytesIO(data))


def unicode_range(codepoints: set[int]) -> str:
    """Collapse code points into a CSS unicode-range list."""
    ranges = []
    for cp in sorted(codepoints):
        if ranges and cp == ranges[-1][1] + 1:
            ranges[-1][1] = cp
        else:
            ranges.append([cp, cp])
    return ",".join(
        f"U+{a:04X}" if a == b else f"U+{a:04X}-{b:04X}" for a, b in ranges
    )


def build(name: str, source: TTFont, codepoints: set[int]) -> set[int]:
    """Write one woff2 subset and return the code points it really maps."""
    font = source
    if LIMIT_WEIGHTS:
        instance = instancer.instantiateVariableFont(
            source, {"wght": WEIGHT_RANGE}, inplace=False
        )
        # The subsetter fails on the instancer's lazily loaded gvar, so
        # reload the instance from bytes first.
        buffer = io.BytesIO()
        instance.save(buffer)
        buffer.seek(0)
        font = TTFont(buffer)
    options = subset.Options()
    options.flavor = "woff2"
    options.layout_features = LAYOUT_FEATURES
    options.hinting = False
    options.drop_tables += ["gasp"]
    options.desubroutinize = True
    options.notdef_outline = True
    subsetter = subset.Subsetter(options)
    subsetter.populate(unicodes=codepoints)
    subsetter.subset(font)
    path = OUT_DIR / f"{name}.woff2"
    subset.save_font(font, str(path), options)
    mapped = set(TTFont(path).getBestCmap())
    print(f"{path.name}: {path.stat().st_size} bytes, {len(mapped)} chars")
    print(f"  unicode-range: {unicode_range(mapped)}")
    return mapped


def main() -> None:
    OUT_DIR.mkdir(exist_ok=True)
    for family, (url, sha256) in SOURCES.items():
        core = build(f"{family}-core", load_source(url, sha256), CORE)
        build(f"{family}-extended", load_source(url, sha256), EXTENDED - core)


if __name__ == "__main__":
    main()
