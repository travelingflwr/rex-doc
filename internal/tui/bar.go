package tui

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/mattn/go-runewidth"

	"github.com/travelingflwr/rex-doc/internal/doc"
)

// The top bar. In Rex it is the one graphical element in rdxt: rounded
// pills, a breadcrumb and a colored rule. Outside Rex it is one dim line.

const (
	capLeft  = "" // powerline round caps; Rex bundles a font with them
	capRight = ""
	barPad   = 2
)

// hit is a clickable part of the top bar.
type hit struct {
	x0, x1 int // columns, end exclusive
	action string
	arg    int
}

// topH is how many rows the top bar takes, including the blank row below it.
func (v *viewer) topH() int {
	if v.rich {
		return 3
	}
	return 2
}

func sgrFG(c doc.Color) string { return doc.Style{FG: c, BG: doc.Default, Link: -1}.SGR() }

// pill draws text on a colored rounded pill and returns it with its width.
func pill(text string, color doc.Color, enabled bool) (string, int) {
	if !enabled {
		color = doc.BrightBlack
	}
	body := doc.Style{FG: doc.Black, BG: color, Bold: enabled, Link: -1}.SGR()
	s := sgrFG(color) + capLeft + "\x1b[0m" + body + text + "\x1b[0m" + sgrFG(color) + capRight + "\x1b[0m"
	return s, runewidth.StringWidth(text) + 2
}

type pillSpec struct {
	label, short string
	color        doc.Color
	action       string
}

// bar returns the top bar's lines and its click targets.
func (v *viewer) bar() ([]string, []hit) {
	if !v.rich {
		return v.plainBar(), nil
	}
	w := v.width
	var hits []hit

	// Right: tools. Short labels when the pane is narrow.
	tools := []pillSpec{
		{" ≡ Outline ", " ≡ ", doc.Cyan, "outline"},
		{" ⌕ Search ", " ⌕ ", doc.Yellow, "search"},
		{" ? ", " ? ", doc.Magenta, "help"},
	}
	// Floating windows can be docked into the tab, and panes floated.
	if v.floating {
		tools = append(tools, pillSpec{" ⇲ Dock ", " ⇲ ", doc.BrightBlue, "dock"},
			pillSpec{" ✕ ", " ✕ ", doc.Red, "close"})
	} else if os.Getenv("REX_BLOCK") != "" {
		tools = append(tools, pillSpec{" ⇱ ", " ⇱ ", doc.BrightBlack, "float"})
	}
	// The mode comes first: View, Source or Insert, with ● for unsaved edits.
	modeP := pillSpec{" View ", " V ", doc.Green, "mode"}
	if v.mode == sourceMode {
		modeP = pillSpec{" Source ", " S ", doc.Blue, "mode"}
		if v.ed.insert {
			modeP = pillSpec{" Insert ", " I ", doc.BrightMagenta, "mode"}
		}
	}
	if v.dirty() {
		modeP.label = strings.TrimRight(modeP.label, " ") + " ● "
		modeP.short = strings.TrimRight(modeP.short, " ") + "● "
	}
	tools = append([]pillSpec{modeP}, tools...)
	compact := w < 72
	var right strings.Builder
	rightW := 0
	type placed struct {
		w      int
		action string
	}
	var rp []placed
	for i, t := range tools {
		label := t.label
		if compact {
			label = t.short
		}
		s, pw := pill(label, t.color, true)
		if i > 0 {
			right.WriteString(" ")
			rightW++
		}
		right.WriteString(s)
		rp = append(rp, placed{pw, t.action})
		rightW += pw
	}

	// Left: Back and Forward, the Rex mark, the breadcrumb.
	var left strings.Builder
	x := barPad
	back, bw := pill(" ‹ ", doc.Blue, len(v.back) > 0)
	left.WriteString(back)
	hits = append(hits, hit{x, x + bw, "back", 0})
	x += bw + 1
	left.WriteString(" ")
	fwd, fw := pill(" › ", doc.Blue, len(v.fwd) > 0)
	left.WriteString(fwd)
	hits = append(hits, hit{x, x + fw, "forward", 0})
	x += fw
	mark := "  " + doc.RexMark + " "
	left.WriteString(mark)
	x += runewidth.StringWidth(mark)

	room := w - barPad*2 - (x - barPad) - rightW - 2
	crumbs, chits := v.breadcrumb(room, x)
	left.WriteString(crumbs)
	hits = append(hits, chits...)
	leftW := x - barPad + visibleWidth(crumbs)

	gap := w - barPad*2 - leftW - rightW
	line1 := strings.Repeat(" ", barPad) + left.String() + strings.Repeat(" ", max(gap, 1)) + right.String()
	rx := barPad + leftW + max(gap, 1)
	for i, p := range rp {
		if i > 0 {
			rx++
		}
		hits = append(hits, hit{rx, rx + p.w, p.action, 0})
		rx += p.w
	}

	// A rule shading from magenta to blue to cyan.
	n := max(w-barPad*2, 0)
	seg := n / 3
	rule := strings.Repeat(" ", barPad) + sgrFG(doc.Magenta) + strings.Repeat("━", seg) + sgrFG(doc.Blue) +
		strings.Repeat("━", seg) + sgrFG(doc.Cyan) + strings.Repeat("━", n-2*seg) + "\x1b[0m"
	return []string{line1, rule, ""}, hits
}

// breadcrumb shows where you've been and where you are, as far as fits in
// room columns. Earlier documents are dim and clickable; the current one is
// bold.
func (v *viewer) breadcrumb(room, x int) (string, []hit) {
	type crumb struct {
		name  string
		steps int // how many times to go back to reach it
	}
	var trail []crumb
	// Collapse repeated visits to the same file (anchor jumps).
	last := v.path
	steps := 0
	for i := len(v.back) - 1; i >= 0; i-- {
		steps++
		if v.back[i].path == last {
			continue
		}
		trail = append([]crumb{{doc.Safe(filepath.Base(v.back[i].path)), steps}}, trail...)
		last = v.back[i].path
	}
	sep := sgrFG(doc.BrightBlack) + " › " + "\x1b[0m"
	current := "\x1b[1m" + runewidth.Truncate(doc.Safe(filepath.Base(v.path)), max(room, 4), "…") + "\x1b[0m"
	used := visibleWidth(current)
	// Add earlier crumbs from the nearest back while they fit.
	var parts []string
	var widths []int
	var cs []crumb
	for i := len(trail) - 1; i >= 0; i-- {
		cw := runewidth.StringWidth(trail[i].name) + 3
		if used+cw > room {
			if used+4 <= room {
				parts = append([]string{sgrFG(doc.BrightBlack) + "…" + "\x1b[0m" + sep}, parts...)
				widths = append([]int{-4}, widths...)
				cs = append([]crumb{{}}, cs...)
			}
			break
		}
		parts = append([]string{sgrFG(doc.BrightBlack) + trail[i].name + "\x1b[0m" + sep}, parts...)
		widths = append([]int{cw}, widths...)
		cs = append([]crumb{trail[i]}, cs...)
		used += cw
	}
	var b strings.Builder
	var hits []hit
	for i, p := range parts {
		b.WriteString(p)
		cw := widths[i]
		if cw < 0 {
			x += -cw
			continue
		}
		hits = append(hits, hit{x, x + cw - 3, "crumb", cs[i].steps})
		x += cw
	}
	b.WriteString(current)
	return b.String(), hits
}

func (v *viewer) plainBar() []string {
	var trail []string
	if len(v.back) > 0 {
		trail = append(trail, filepath.Base(v.back[len(v.back)-1].path))
	}
	left := filepath.Base(v.path)
	if len(trail) > 0 && trail[0] != left {
		left = "‹ " + trail[0] + " › " + left
	}
	right := "o outline · / search · ? help"
	room := v.width - barPad*2 - runewidth.StringWidth(right) - 2
	if room < 10 {
		right = "? help"
		room = v.width - barPad*2 - runewidth.StringWidth(right) - 2
	}
	left = runewidth.Truncate(doc.Safe(left), max(room, 1), "…")
	gap := v.width - barPad*2 - runewidth.StringWidth(left) - runewidth.StringWidth(right)
	return []string{"\x1b[90m" + strings.Repeat(" ", barPad) + left + strings.Repeat(" ", max(gap, 1)) + right + "\x1b[0m", ""}
}

// visibleWidth is the width of s without its escape sequences.
func visibleWidth(s string) int {
	var b strings.Builder
	esc := false
	for _, r := range s {
		switch {
		case esc:
			if r >= 0x40 && r <= 0x7e && r != '[' {
				esc = false
			}
		case r == 0x1b:
			esc = true
		default:
			b.WriteRune(r)
		}
	}
	return runewidth.StringWidth(b.String())
}

// barClick runs what's under column x of the top bar. was is the state
// before the click, so a second click on Outline or ? closes what it opened.
func (v *viewer) barClick(x int, was state) {
	for _, h := range v.hits {
		if x >= h.x0 && x < h.x1 {
			switch h.action {
			case "back":
				v.goBack()
			case "forward":
				v.goForward()
			case "mode":
				if v.mode == viewMode {
					v.enterSource()
				} else {
					if v.ed.insert {
						v.ed.insert = false
						v.ed.clampCursor()
					}
					v.leaveSource()
				}
			case "outline":
				if was != outlining {
					v.openOutline()
				}
			case "search":
				v.state, v.typing = searching, ""
			case "help":
				if was != helping {
					v.state = helping
				}
			case "close":
				v.quit = true
			case "dock":
				if rexDock() {
					v.floating = false
				} else {
					v.setFlash("couldn't dock here")
				}
			case "float":
				if rexFloat() {
					v.floating = true
				} else {
					v.setFlash("couldn't float this pane")
				}
			case "crumb":
				for i := 0; i < h.arg && len(v.back) > 0; i++ {
					v.goBack()
				}
			}
			return
		}
	}
}

// The help screen, opened with ? or the ? pill.
var helpKeys = [][2]string{
	{"j k  ↓ ↑", "scroll a line"},
	{"Space b", "scroll a page"},
	{"Ctrl-d Ctrl-u", "half a page"},
	{"gg G", "top, end"},
	{"} {", "next, previous heading"},
	{"o", "outline"},
	{"Tab Shift-Tab", "select a link"},
	{"Enter, click", "follow the link"},
	{"f", "link hints"},
	{"H L", "back, forward"},
	{"/  n N", "search, next, previous"},
	{"m", "show frontmatter"},
	{"e  Esc", "Source mode, back to View"},
	{"E", "edit in Neovim"},
	{"tap ( )", "tick a task (u undoes)"},
	{"r", "reload"},
	{"Esc", "clear selection; closes a floating window"},
	{"q", "quit"},
}

func (v *viewer) drawHelp(b *strings.Builder, rows int) {
	margin := strings.Repeat(" ", v.doc.Margin)
	b.WriteString("\x1b[2K" + margin + "\x1b[1mKeys\x1b[0m\x1b[90m  Esc or ? to close\x1b[0m\r\n")
	b.WriteString("\x1b[2K\r\n")
	for r := 0; r < rows-2; r++ {
		b.WriteString("\x1b[2K")
		if r < len(helpKeys) {
			k := helpKeys[r]
			b.WriteString(margin + "\x1b[1m" + runewidth.FillRight(k[0], 16) + "\x1b[0m" + k[1])
		} else if r == len(helpKeys)+1 {
			b.WriteString(margin + "\x1b[90m★ Like rdxt? A star helps: github.com/travelingflwr/rex-doc\x1b[0m")
		}
		b.WriteString("\r\n")
	}
}
