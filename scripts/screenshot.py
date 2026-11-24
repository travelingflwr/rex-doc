#!/usr/bin/env python3
"""Make a README screenshot of rdxt running in Rex.

Opens rdxt on FILE in a new Rex tab, optionally sends keys, captures the
pane as HTML (rex capture --format=html), and renders it to a PNG with
headless Chrome, in a One Dark palette and a Nerd Font.

Usage: screenshot.py OUT.png FILE [--keys KEYS] [--wait SECONDS]

Figures drawn with kitty graphics don't survive the HTML export, so their
cells are left blank; take screenshots of figures by hand.
"""
import argparse, html, os, re, subprocess, sys, tempfile, time

CHROME = "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome"
FONT = "FiraCode Nerd Font Mono"
FG, BG = "#abb2bf", "#1b1f27"
PALETTE = ["#282c34", "#e06c75", "#98c379", "#e5c07b", "#61afef", "#c678dd", "#56b6c2", "#abb2bf",
           "#5c6370", "#e06c75", "#98c379", "#e5c07b", "#61afef", "#c678dd", "#56b6c2", "#ffffff"]


def rex(*args):
    return subprocess.run(["rex", *args], capture_output=True, text=True, check=True).stdout.strip()


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("out")
    ap.add_argument("file")
    ap.add_argument("--keys", default="", help="keys to send after opening, e.g. 'e' or '/needle\\r'")
    ap.add_argument("--wait", type=float, default=1.5)
    a = ap.parse_args()

    session = os.environ.get("REX_SESSION") or os.environ.get("REX_BLOCK")
    # Prefer this repo's build, so screenshots show the code being worked on.
    local = os.path.join(os.path.dirname(os.path.abspath(__file__)), "..", "rdxt")
    rdxt = os.path.abspath(local) if os.path.exists(local) else (
        subprocess.run(["which", "rdxt"], capture_output=True, text=True).stdout.strip() or "rdxt")
    block = rex("run", "-s", session, "--focus=false", "--", rdxt, os.path.abspath(a.file))
    block = re.search(r"block:[0-9a-f-]+", block).group(0)
    try:
        time.sleep(a.wait)
        if a.keys:
            rex("send", "-b", block, "--", a.keys.encode().decode("unicode_escape"))
            time.sleep(0.8)
        captured = rex("capture", "-b", block, "--format=html")
        rows = captured.count("\n") + 1
    finally:
        subprocess.run(["rex", "block", "close", block], capture_output=True)

    # Leave kitty placeholder cells (U+10EEEE and its diacritics) blank.
    captured = re.sub(r"&#1109742;(&#7[0-9]{2};)*", " ", captured)
    cols = max(len(html.unescape(re.sub(r"<[^>]+>", "", line))) for line in captured.split("\n"))
    palette = "".join(f"--vt-palette-{i}:{c};" for i, c in enumerate(PALETTE))
    page = f"""<!doctype html><meta charset="utf-8"><style>
:root{{{palette}}}
html,body{{margin:0;background:#0d0f13}}
.win{{display:inline-block;margin:28px;border-radius:12px;background:{BG};color:{FG};
  box-shadow:0 18px 50px rgba(0,0,0,.55),0 0 0 1px rgba(255,255,255,.06);overflow:hidden}}
.bar{{height:30px;display:flex;align-items:center;gap:8px;padding:0 14px;background:#15181e}}
.dot{{width:12px;height:12px;border-radius:50%}}
.term{{padding:14px 18px 18px;font:15px/1.32 "{FONT}",monospace}}
.term > div{{font-family:inherit !important}}
</style><div class="win"><div class="bar"><span class="dot" style="background:#ff5f57"></span>
<span class="dot" style="background:#febc2e"></span><span class="dot" style="background:#28c840"></span></div>
<div class="term">{captured}</div></div>"""
    with tempfile.NamedTemporaryFile("w", suffix=".html", delete=False) as f:
        f.write(page)
        path = f.name
    width = int(cols * 9.05) + 36 + 56
    height = int(rows * 15 * 1.32) + 30 + 32 + 56
    subprocess.run([CHROME, "--headless=new", "--hide-scrollbars", "--force-device-scale-factor=2",
                    f"--window-size={width},{height}", f"--screenshot={os.path.abspath(a.out)}",
                    "file://" + path], capture_output=True, check=True)
    os.unlink(path)
    print(a.out)


if __name__ == "__main__":
    main()
