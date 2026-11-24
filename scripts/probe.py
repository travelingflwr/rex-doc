#!/usr/bin/env python3
"""Probe what the current terminal supports, for rdxt.

Part 1 queries the terminal automatically (text sizing, colors, kitty graphics,
kitty keyboard, version). Part 2 shows a visual sample. Part 3 logs mouse events
until you press q.

Usage: probe.py RESULTS.json
"""
import json, os, re, select, sys, termios, time, tty

OUT = sys.argv[1] if len(sys.argv) > 1 else "probe-results.json"
ESC = "\x1b"
fd = sys.stdin.fileno()
results = {"env": {k: os.environ.get(k) for k in ("TERM", "TERM_PROGRAM", "TERM_PROGRAM_VERSION", "REX_SESSION", "REX_BLOCK", "COLORTERM")}}


def w(s):
    sys.stdout.write(s)
    sys.stdout.flush()


def read_until(pred, timeout=0.6):
    buf = b""
    end = time.time() + timeout
    while time.time() < end:
        r, _, _ = select.select([fd], [], [], max(0, end - time.time()))
        if not r:
            break
        buf += os.read(fd, 4096)
        if pred(buf):
            break
    return buf.decode("utf-8", "replace")


def query(seq, done=r"\x1b\[\?[\d;]*c"):
    """Send seq followed by DA1, read until the DA1 reply comes back."""
    w(seq + ESC + "[c")
    return read_until(lambda b: re.search(done.encode(), b) is not None)


def cursor():
    resp = query(ESC + "[6n")
    m = re.search(r"\x1b\[(\d+);(\d+)R", resp)
    return (int(m.group(1)), int(m.group(2))) if m else None


old = termios.tcgetattr(fd)
try:
    tty.setraw(fd)
    w(ESC + "[2J" + ESC + "[H")

    # --- Text sizing (OSC 66): does a w=2 space advance the cursor 2 cells?
    w("\r")
    before = cursor()
    w(ESC + "]66;w=2; \x07")
    after = cursor()
    results["osc66_width_test"] = {"before": before, "after": after}
    results["osc66_supported"] = bool(before and after and after[1] - before[1] == 2)
    w("\r" + ESC + "[2K")

    # --- Colors
    for name, seq in (("bg", "]11;?"), ("fg", "]10;?")):
        resp = query(ESC + seq + "\x07")
        m = re.search(r"rgb:([0-9a-fA-F/]+)", resp)
        results[name] = m.group(1) if m else None

    # --- Kitty graphics
    resp = query(ESC + "_Gi=31,s=1,v=1,a=q,t=d,f=24;AAAA" + ESC + "\\")
    results["kitty_graphics"] = "OK" in resp

    # --- Kitty keyboard protocol
    resp = query(ESC + "[?u")
    results["kitty_keyboard"] = bool(re.search(r"\x1b\[\?\d+u", resp))

    # --- Terminal version (XTVERSION)
    resp = query(ESC + "[>0q")
    m = re.search(r"\x1bP>\|(.*?)\x1b\\", resp)
    results["xtversion"] = m.group(1) if m else None

    # --- Cell size in pixels
    resp = query(ESC + "[16t")
    m = re.search(r"\x1b\[6;(\d+);(\d+)t", resp)
    results["cell_px"] = {"h": int(m.group(1)), "w": int(m.group(2))} if m else None

    # --- Synchronized output (DECRQM 2026)
    resp = query(ESC + "[?2026$p")
    m = re.search(r"\x1b\[\?2026;(\d)\$y", resp)
    results["sync_output"] = m.group(1) if m else None

    with open(OUT, "w") as f:
        json.dump(results, f, indent=2)

    # --- Part 2: visual sample
    nl = "\r\n"
    w(ESC + "[2J" + ESC + "[H")
    w(ESC + "[1mrdxt terminal probe" + ESC + "[0m   (automatic results saved)" + nl + nl)
    w("OSC 66 supported: " + str(results["osc66_supported"]) + nl + nl)
    w(ESC + "]66;s=2;" + ESC + "[1mHeading 1 at 2x" + ESC + "[0m\x07" + nl + nl)
    w(ESC + "]66;s=2:n=3:d=4;" + ESC + "[1mHeading 2 at 1.5x" + ESC + "[0m\x07" + nl + nl)
    w(ESC + "[1;35mHeading 1 (fallback: bold + color)" + ESC + "[0m" + nl)
    w(ESC + "[35m" + "━" * 34 + ESC + "[0m" + nl + nl)
    w("Normal  " + ESC + "[1mbold" + ESC + "[0m  " + ESC + "[3mitalic" + ESC + "[0m  " + ESC + "[2mdim" + ESC + "[0m  "
      + ESC + "[9mstrike" + ESC + "[0m  " + ESC + "[4:3;58;5;4mcurly blue underline" + ESC + "[0m" + nl)
    w("Link: " + ESC + "]8;;https://example.com" + ESC + "\\" + ESC + "[4;34mexample.com (OSC 8)" + ESC + "[0m" + ESC + "]8;;" + ESC + "\\" + nl)
    w("Palette: " + "".join(ESC + "[48;5;%dm  " % i for i in range(16)) + ESC + "[0m" + nl)
    w("Box: ┌─┬─┐ │▎│ └─┴─┘  ☐ ☑  ▸ ▾  •  ›" + nl + nl)

    # --- Part 3: mouse
    w(ESC + "[1mMouse test:" + ESC + "[0m click, shift-click, option-click, cmd-click, drag, scroll, tap and swipe (iPad)." + nl)
    w("Press q to finish." + nl + nl)
    w(ESC + "[?1000h" + ESC + "[?1002h" + ESC + "[?1006h")
    events = []
    buf = ""
    deadline = time.time() + 180
    while time.time() < deadline:
        r, _, _ = select.select([fd], [], [], 0.5)
        if not r:
            continue
        buf += os.read(fd, 4096).decode("utf-8", "replace")
        for m in re.finditer(r"\x1b\[<(\d+);(\d+);(\d+)([Mm])", buf):
            b, x, y, kind = int(m.group(1)), int(m.group(2)), int(m.group(3)), m.group(4)
            mods = [n for bit, n in ((4, "shift"), (8, "alt"), (16, "ctrl")) if b & bit]
            base = b & ~(4 | 8 | 16)
            what = {0: "left", 1: "middle", 2: "right", 64: "wheel-up", 65: "wheel-down", 66: "wheel-left", 67: "wheel-right"}.get(base & ~32, str(base))
            if base & 32:
                what += "-drag"
            ev = {"btn": what, "mods": mods, "x": x, "y": y, "press": kind == "M"}
            events.append(ev)
            w("  %-12s %-14s at %3d,%-3d %s" % (what, "+".join(mods) or "-", x, y, "down" if kind == "M" else "up") + nl)
        if "q" in re.sub(r"\x1b\[<[\d;]+[Mm]", "", buf):
            break
        buf = buf[buf.rfind("\x1b"):] if "\x1b" in buf and not buf.endswith(("M", "m")) else ""
    w(ESC + "[?1006l" + ESC + "[?1002l" + ESC + "[?1000l")
    results["mouse_events"] = events
    with open(OUT, "w") as f:
        json.dump(results, f, indent=2)
    w(nl + "Done. You can close this pane." + nl)
finally:
    termios.tcsetattr(fd, termios.TCSADRAIN, old)
