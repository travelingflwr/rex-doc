// Package tui runs rdxt's full-screen viewer.
package tui

import (
	"bytes"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/mattn/go-runewidth"
	"golang.org/x/term"

	"github.com/travelingflwr/rex-doc/internal/doc"
	"github.com/travelingflwr/rex-doc/internal/rex"
)

// What the viewer is doing with the keyboard.
type state int

const (
	reading   state = iota
	searching       // typing a search
	hinting         // link hints are shown
	outlining       // the outline is shown
	helping         // the help screen is shown
)

// place is a spot in a file, for Back and Forward.
type place struct {
	path string
	src  int // source offset of the top line
}

type viewer struct {
	path     string
	src      []byte
	mod      time.Time
	size     int64
	doc      *doc.Doc
	top      int
	width    int
	height   int
	out      *os.File
	flash    string
	flashT   time.Time
	flashFor time.Duration
	// scold shows the flash in red (Bad Rexy).
	scold bool
	// pending holds a key that starts a two-key command, like "g" in "gg".
	pending string
	// showFM shows the frontmatter card (toggled with m).
	showFM bool

	state state
	// sel is the selected link (index into doc.Links), or -1.
	sel int
	// back and fwd are the history stacks.
	back, fwd []place

	// Search.
	query   string
	typing  string
	matches []match
	cur     int

	// Link hints: label → link index.
	hints map[string]int
	hint  string

	// Outline: the selected heading.
	osel int

	// rich is true in Rex, where the top bar is graphical.
	rich bool
	// floating is true when Rex shows rdxt in a floating layer.
	floating bool
	// leftSource is true from leaving Source mode until the next key.
	leftSource bool
	// check is Rex's check of its config, when that's the file shown.
	check *doc.Check
	// hits are the top bar's click targets.
	hits []hit
	// quit is set by the close pill.
	quit bool

	// mode is View or Source; ed is Source mode's buffer.
	mode mode
	ed   *editor
	// hl caches Source mode's highlighting for hlText.
	hl     [][]doc.Span
	hlText string
	// viewUndo holds the file before each edit made from View mode.
	viewUndo [][]byte
	// diskChanged is set when the file changed on disk under unsaved edits.
	diskChanged bool

	inFd     int
	oldState *term.State

	// images tracks figures' scaled copies (in Rex); cellW and cellH are the
	// cell size in pixels.
	images       *imageState
	cellW, cellH int
	// diagrams is the last doc.DiagramsReady seen.
	diagrams int64
}

type match struct {
	line, start, end int
}

// RunOptions change how the viewer starts.
type RunOptions struct {
	// Source starts in Source mode, ready to edit (for new documents).
	Source bool
}

// Run shows path full screen until the user quits.
func Run(path string) error { return RunWith(path, RunOptions{}) }

// RunWith is Run with options.
func RunWith(path string, opts RunOptions) error {
	in, out := os.Stdin, os.Stdout
	if !term.IsTerminal(int(in.Fd())) {
		tty, err := os.Open("/dev/tty")
		if err != nil {
			return fmt.Errorf("no terminal to read keys from: %w", err)
		}
		defer tty.Close()
		in = tty
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	v := &viewer{path: abs, out: out, sel: -1, rich: inRex()}
	if err := v.load(); err != nil {
		return err
	}
	old, err := term.MakeRaw(int(in.Fd()))
	if err != nil {
		return err
	}
	defer term.Restore(int(in.Fd()), old)
	v.inFd, v.oldState = int(in.Fd()), old

	// Alternate screen, hidden cursor, mouse clicks and wheel (SGR encoding).
	v.write("\x1b[?1049h\x1b[?25l\x1b[?1000h\x1b[?1006h")
	defer v.write("\x1b[?1006l\x1b[?1000l\x1b[0 q\x1b[?25h\x1b[?1049l")

	if v.rich {
		v.images = newImageState()
		v.cellW, v.cellH = cellSize(int(out.Fd()))
		defer v.write("\x1b_Ga=d,d=A,q=2\x1b\\")
	}
	v.resize()
	recordRecent(v.path)
	if opts.Source {
		v.enterSource()
	}
	if launches() == starAskLaunch {
		v.flash, v.flashT, v.flashFor = "★ Enjoying rdxt? A star helps: github.com/travelingflwr/rex-doc", time.Now(), 8*time.Second
	}
	v.draw()

	// Ask Rex whether we're floating, after the first screen is up.
	floatCh := make(chan bool, 1)
	if v.rich {
		rexLabel(v.path)
		go func() { floatCh <- rexFloating() }()
	}

	keys := make(chan []byte, 16)
	go func() {
		buf := make([]byte, 4096)
		for {
			n, err := in.Read(buf)
			if err != nil {
				close(keys)
				return
			}
			keys <- append([]byte(nil), buf[:n]...)
		}
	}()
	winch := make(chan os.Signal, 1)
	signal.Notify(winch, syscall.SIGWINCH)
	defer signal.Stop(winch)
	tick := time.NewTicker(400 * time.Millisecond)
	defer tick.Stop()
	var imgReady chan thumb
	if v.images != nil {
		imgReady = v.images.ready
	}

	for {
		select {
		case b, ok := <-keys:
			if !ok {
				return nil
			}
			if v.input(b) {
				return nil
			}
		case f := <-floatCh:
			v.floating = f
		case t := <-imgReady:
			v.sendImage(t)
		case <-winch:
			v.resize()
		case <-tick.C:
			// Some terminals (Rex on iPad) don't always send SIGWINCH, so
			// check the size here too.
			if w, h, err := term.GetSize(int(v.out.Fd())); err == nil && (w != v.width || h != v.height) {
				v.resize()
			}
			v.checkReload()
			// A Mermaid picture finished drawing in the background.
			if n := doc.DiagramsReady.Load(); n != v.diagrams {
				v.diagrams = n
				v.relayout()
			}
			if v.flash != "" && time.Since(v.flashT) > max(v.flashFor, 2*time.Second) {
				v.flash = ""
			}
		}
		v.draw()
	}
}

func (v *viewer) write(s string) { v.out.WriteString(s) }

func (v *viewer) load() error {
	st, err := os.Stat(v.path)
	if err != nil {
		return err
	}
	src, err := os.ReadFile(v.path)
	if err != nil {
		return err
	}
	v.src, v.mod, v.size = src, st.ModTime(), st.Size()
	v.check = nil
	if rex.IsConfig(v.path) {
		v.check = rex.CheckConfig()
	}
	return nil
}

// relayout lays the document out again, keeping the reader's place.
func (v *viewer) relayout() {
	anchor := -1
	if v.doc != nil && v.top > 0 {
		anchor = v.doc.SrcForLine(v.top)
	}
	opts := doc.OptionsFor(v.path)
	opts.Check = v.check
	opts.ShowFrontmatter = v.showFM
	opts.Images = v.rich
	if v.cellW > 0 {
		opts.CellW, opts.CellH = v.cellW, v.cellH
	}
	opts.ScreenRows = v.height
	v.doc = doc.Render(v.src, v.width, opts)
	v.forgetImages()
	if anchor >= 0 {
		v.top = v.doc.LineForSrc(anchor)
	}
	if v.sel >= len(v.doc.Links) {
		v.sel = -1
	}
	if v.query != "" {
		v.search(v.query, false)
	}
	v.clamp()
}

func (v *viewer) resize() {
	w, h, err := term.GetSize(int(v.out.Fd()))
	if err != nil || w <= 0 {
		w, h = 80, 24
	}
	v.width, v.height = w, h
	v.relayout()
}

func (v *viewer) checkReload() {
	st, err := os.Stat(v.path)
	if err != nil || (st.ModTime().Equal(v.mod) && st.Size() == v.size) {
		return
	}
	if v.dirty() {
		// Don't lose unsaved edits; :w will overwrite the file.
		if !v.diskChanged {
			v.diskChanged = true
			v.setFlash("file changed on disk · :w overwrites it")
		}
		return
	}
	v.diskChanged = false
	if v.load() == nil {
		if v.ed != nil {
			row, col, top := v.ed.row, v.ed.col, v.ed.top
			v.ed.set(string(v.src))
			v.ed.row, v.ed.col, v.ed.top = row, col, top
			v.ed.clampCursor()
		}
		v.relayout()
		v.setFlash("reloaded")
	}
}

func (v *viewer) setFlash(s string) {
	v.flash, v.flashT, v.flashFor, v.scold = s, time.Now(), 0, false
}

// rows is the number of document rows: the screen less the status line and
// the blank line above it.
func (v *viewer) rows() int { return max(v.height-v.topH()-2, 1) }

func (v *viewer) maxTop() int { return max(len(v.doc.Lines)-v.rows(), 0) }

func (v *viewer) clamp() { v.top = min(max(v.top, 0), v.maxTop()) }

func (v *viewer) scroll(n int) { v.top += n; v.clamp() }

// show scrolls so line i is visible, a few lines from the top if it wasn't.
func (v *viewer) show(i int) {
	if i < v.top || i >= v.top+v.rows() {
		v.top = i - min(3, v.rows()/4)
		v.clamp()
	}
}

var mouseRE = regexp.MustCompile(`^\x1b\[<(\d+);(\d+);(\d+)([Mm])`)

// input handles a chunk of input and reports whether to quit.
func (v *viewer) input(b []byte) bool {
	for len(b) > 0 {
		if m := mouseRE.FindSubmatch(b); m != nil {
			btn, _ := strconv.Atoi(string(m[1]))
			x, _ := strconv.Atoi(string(m[2]))
			y, _ := strconv.Atoi(string(m[3]))
			v.mouse(btn, x-1, y-1, m[4][0] == 'M')
			b = b[len(m[0]):]
			continue
		}
		key, rest := nextKey(b)
		b = rest
		if v.key(key) {
			return true
		}
	}
	return v.quit
}

func (v *viewer) mouse(btn, x, y int, press bool) {
	top := v.topH()
	if btn&^(4|8|16) == 0 && !press && y < top {
		if v.state == reading || v.state == helping || v.state == outlining {
			was := v.state
			v.state = reading
			v.barClick(x, was)
		}
		return
	}
	y -= top
	if v.mode == sourceMode && v.state == reading {
		switch btn &^ (4 | 8 | 16) {
		case 64:
			v.ed.moveRow(-3)
		case 65:
			v.ed.moveRow(3)
		case 0:
			if !press && y < v.rows() {
				v.sourceClick(x, y)
			}
		}
		return
	}
	switch btn &^ (4 | 8 | 16) {
	case 64:
		if v.state == outlining {
			v.osel = max(v.osel-1, 0)
		} else {
			v.scroll(-3)
		}
	case 65:
		if v.state == outlining {
			v.osel = min(v.osel+1, len(v.doc.Headings)-1)
		} else {
			v.scroll(3)
		}
	case 0:
		if press {
			return
		}
		// A click (on release) follows a link.
		if v.state == outlining {
			if i := v.outlineTop() + y - 2; i >= 0 && i < len(v.doc.Headings) {
				v.osel = i
				v.key("\r")
			}
			return
		}
		if y >= v.rows() {
			return
		}
		// A tap on a task's ( ) ticks it.
		if i := v.top + y; i < len(v.doc.Lines) && v.doc.Lines[i].TaskCols > 0 {
			if c := x - v.doc.Margin; c >= 0 && c < v.doc.Lines[i].TaskCols {
				v.toggleTask(i)
				return
			}
		}
		if l := v.doc.LinkAt(v.top+y, x-v.doc.Margin); l >= 0 {
			v.sel = l
			v.follow(l)
		}
	}
}

// Escape sequences for keys rdxt uses.
var seqs = map[string]string{
	"\x1b[A": "up", "\x1b[B": "down", "\x1bOA": "up", "\x1bOB": "down",
	"\x1b[C": "right", "\x1b[D": "left", "\x1bOC": "right", "\x1bOD": "left", "\x1b[3~": "delete",
	"\x1b[1;2A": "shift-up", "\x1b[1;2B": "shift-down",
	// Shift+Enter, however the terminal sends it, is Enter.
	"\x1b\r": "\r", "\x1b[13;2u": "\r", "\x1b[27;2;13~": "\r", "\x1b\n": "\r",
	"\x1b[5~": "pgup", "\x1b[6~": "pgdn", "\x1b[Z": "shift-tab",
	"\x1b[H": "home", "\x1b[F": "end", "\x1b[1~": "home", "\x1b[4~": "end", "\x1bOH": "home", "\x1bOF": "end",
}

func nextKey(b []byte) (string, []byte) {
	if b[0] == 0x1b {
		for s, name := range seqs {
			if bytes.HasPrefix(b, []byte(s)) {
				return name, b[len(s):]
			}
		}
		if len(b) == 1 {
			return "esc", nil
		}
		// An unknown sequence: skip to its final byte.
		if b[1] == '[' || b[1] == 'O' {
			for i := 2; i < len(b); i++ {
				if b[i] >= 0x40 && b[i] <= 0x7e {
					return "", b[i+1:]
				}
			}
			return "", nil
		}
		return "esc", b[1:]
	}
	// Enter can arrive as CR, LF or CR LF depending on the terminal.
	if b[0] == '\r' || b[0] == '\n' {
		if b[0] == '\r' && len(b) > 1 && b[1] == '\n' {
			return "\r", b[2:]
		}
		return "\r", b[1:]
	}
	r := []rune(string(b))
	return string(r[0]), b[len(string(r[0])):]
}

func (v *viewer) key(k string) bool {
	switch v.state {
	case searching:
		v.searchKey(k)
		return false
	case hinting:
		v.hintKey(k)
		return false
	case outlining:
		v.outlineKey(k)
		return false
	case helping:
		if k == "esc" || k == "?" || k == "q" || k == "\x03" {
			v.state = reading
		}
		return false
	}
	if v.mode == sourceMode {
		return v.sourceKey(k)
	}
	leftSource := v.leftSource
	v.leftSource = false
	if v.pending == "g" {
		v.pending = ""
		if k == "g" {
			v.top = 0
			return false
		}
	}
	half := max(v.rows()/2, 1)
	page := max(v.rows()-2, 1)
	switch k {
	case "q", "\x03":
		if v.dirty() {
			v.setFlash("unsaved changes: e then :w to save, or :q! to discard")
			return false
		}
		return true
	case "e":
		v.enterSource()
	case "E":
		line := 1
		if v.top > 0 {
			line = bytes.Count(v.src[:min(v.doc.SrcForLine(v.top), len(v.src))], []byte("\n")) + 1
		}
		v.openNeovim(line)
	case "u":
		v.undoView()
	case "esc":
		// In a floating window, Esc with nothing to clear closes it: a
		// quick peek, gone as fast as it came.
		// Not right after Esc left Source mode, so a double Esc stays.
		if v.floating && !leftSource && v.sel < 0 && v.query == "" && !v.dirty() {
			return true
		}
		v.sel = -1
		v.query, v.matches = "", nil
	case "j", "down", "\x05":
		v.scroll(1)
	case "k", "up", "\x19":
		v.scroll(-1)
	case "\x04", "shift-down":
		v.scroll(half)
	case "\x15", "shift-up":
		v.scroll(-half)
	case " ", "pgdn", "\x06":
		v.scroll(page)
	case "b", "pgup", "\x02":
		v.scroll(-page)
	case "g":
		v.pending = "g"
	case "home":
		v.top = 0
	case "G", "end":
		v.top = v.maxTop()
	case "}":
		v.jumpHeading(1)
	case "{":
		v.jumpHeading(-1)
	case "\t":
		v.nextLink(1)
	case "shift-tab":
		v.nextLink(-1)
	case "\r":
		if v.sel >= 0 {
			v.follow(v.sel)
		}
	case "f":
		v.startHints()
	case "H", "\x0f", "\x7f":
		v.goBack()
	case "L":
		v.goForward()
	case "/":
		v.state, v.typing = searching, ""
	case "n":
		v.nextMatch(1)
	case "N":
		v.nextMatch(-1)
	case "o":
		v.openOutline()
	case "?":
		v.state = helping
	case "m":
		if v.doc.HasFrontmatter {
			v.showFM = !v.showFM
			v.relayout()
			if v.showFM {
				v.top = 0
			}
		}
	case "r":
		if v.load() == nil {
			v.relayout()
			v.setFlash("reloaded")
		}
	}
	return false
}

// jumpHeading moves to the next (dir 1) or previous (dir -1) heading.
func (v *viewer) jumpHeading(dir int) {
	hs := v.doc.Headings
	if dir > 0 {
		for _, h := range hs {
			if h.Line > v.top {
				v.top = h.Line
				v.clamp()
				return
			}
		}
		return
	}
	for i := len(hs) - 1; i >= 0; i-- {
		if hs[i].Line < v.top {
			v.top = hs[i].Line
			v.clamp()
			return
		}
	}
	v.top = 0
}

// ---- Links ----

// nextLink selects the next (dir 1) or previous (dir -1) link. With nothing
// selected it starts from the top of the screen.
func (v *viewer) nextLink(dir int) {
	runs := v.doc.LinkRuns()
	if len(runs) == 0 {
		v.setFlash("no links")
		return
	}
	// One entry per link, at its first run.
	var order []doc.LinkRun
	seen := map[int]bool{}
	for _, r := range runs {
		if !seen[r.Link] {
			seen[r.Link] = true
			order = append(order, r)
		}
	}
	idx := -1
	for i, r := range order {
		if r.Link == v.sel {
			idx = i
		}
	}
	// If the selection is off screen, start again from what's visible.
	if idx >= 0 && (order[idx].Line < v.top || order[idx].Line >= v.top+v.rows()) {
		idx = -1
	}
	if idx < 0 {
		for i, r := range order {
			if r.Line >= v.top {
				idx = i
				break
			}
		}
		if idx < 0 {
			idx = len(order) - 1
		}
		if dir < 0 && order[idx].Line >= v.top {
			idx = (idx - 1 + len(order)) % len(order)
		}
		if dir > 0 && v.sel == -1 {
			// The first visible link.
		} else if dir > 0 {
			idx = (idx + 1) % len(order)
		}
	} else {
		idx = (idx + dir + len(order)) % len(order)
	}
	v.sel = order[idx].Link
	v.show(order[idx].Line)
}

// follow opens link l.
func (v *viewer) follow(l int) {
	dest := strings.TrimSpace(v.doc.Links[l].Dest)
	switch {
	case dest == "":
		return
	case v.doc.Links[l].Image && !isURL(dest):
		v.showImage(dest)
	case strings.HasPrefix(dest, "#"):
		v.jumpAnchor(dest[1:], true)
	case isURL(dest):
		if err := openExternal(dest); err != nil {
			v.setFlash("can't open " + dest)
		} else {
			v.setFlash("opened in browser")
		}
	default:
		file, anchor, _ := strings.Cut(dest, "#")
		if p, err := pathUnescape(file); err == nil {
			file = p
		}
		if !filepath.IsAbs(file) {
			file = filepath.Join(filepath.Dir(v.path), file)
		}
		st, err := os.Stat(file)
		if err != nil {
			v.setFlash("not found: " + filepath.Base(file))
			return
		}
		if st.IsDir() {
			for _, name := range []string{"README.md", "readme.md", "index.md"} {
				if _, err := os.Stat(filepath.Join(file, name)); err == nil {
					file = filepath.Join(file, name)
					st = nil
					break
				}
			}
			if st != nil {
				v.openOther(file)
				return
			}
		}
		if isDocument(file) {
			v.visit(file, anchor)
			return
		}
		v.openOther(file)
	}
}

func isURL(s string) bool {
	for _, p := range []string{"http://", "https://", "mailto:", "ftp://"} {
		if strings.HasPrefix(strings.ToLower(s), p) {
			return true
		}
	}
	return false
}

// isDocument reports whether rdxt shows a file itself: Markdown, Dok and
// plain text. Links to them open in place.
func isDocument(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".md", ".markdown", ".mdown", ".mkd", ".mdx", ".dok", ".txt":
		return true
	}
	return false
}

// visit opens another Markdown file in this pane, remembering where we were.
func (v *viewer) visit(file, anchor string) {
	here := place{v.path, v.doc.SrcForLine(v.top)}
	if file != v.path {
		old := v.path
		v.path = file
		if err := v.load(); err != nil {
			v.path = old
			v.setFlash("can't open " + filepath.Base(file))
			return
		}
		v.back = append(v.back, here)
		v.fwd = nil
		v.reset()
		if v.rich {
			rexLabel(v.path)
		}
	} else {
		v.back = append(v.back, here)
		v.fwd = nil
	}
	if anchor != "" {
		v.jumpAnchor(anchor, false)
	}
}

// reset clears per-document state after switching files.
func (v *viewer) reset() {
	v.top, v.sel, v.showFM = 0, -1, false
	v.query, v.matches = "", nil
	v.doc = nil
	v.relayout()
}

func (v *viewer) jumpAnchor(slug string, remember bool) {
	line := v.doc.HeadingBySlug(slug)
	if line < 0 {
		v.setFlash("no heading #" + slug)
		return
	}
	if remember {
		v.back = append(v.back, place{v.path, v.doc.SrcForLine(v.top)})
		v.fwd = nil
	}
	v.top = line
	v.clamp()
}

func (v *viewer) goBack() {
	if len(v.back) == 0 {
		v.setFlash("nothing to go back to")
		return
	}
	p := v.back[len(v.back)-1]
	v.back = v.back[:len(v.back)-1]
	v.fwd = append(v.fwd, place{v.path, v.doc.SrcForLine(v.top)})
	v.goTo(p)
}

func (v *viewer) goForward() {
	if len(v.fwd) == 0 {
		v.setFlash("nothing to go forward to")
		return
	}
	p := v.fwd[len(v.fwd)-1]
	v.fwd = v.fwd[:len(v.fwd)-1]
	v.back = append(v.back, place{v.path, v.doc.SrcForLine(v.top)})
	v.goTo(p)
}

func (v *viewer) goTo(p place) {
	if p.path != v.path {
		v.path = p.path
		if err := v.load(); err != nil {
			v.setFlash("can't open " + filepath.Base(p.path))
			return
		}
		v.reset()
		if v.rich {
			rexLabel(v.path)
		}
	}
	v.top = v.doc.LineForSrc(p.src)
	if p.src == 0 {
		v.top = 0
	}
	v.clamp()
}

// ---- Link hints ----

const hintKeys = "asdfghjklqwertyuiopzxcvbnm"

func (v *viewer) startHints() {
	runs := v.doc.LinkRuns()
	var visible []int
	seen := map[int]bool{}
	for _, r := range runs {
		if r.Line >= v.top && r.Line < v.top+v.rows() && !seen[r.Link] {
			seen[r.Link] = true
			visible = append(visible, r.Link)
		}
	}
	if len(visible) == 0 {
		v.setFlash("no links on screen")
		return
	}
	v.hints = map[string]int{}
	for i, l := range visible {
		v.hints[hintLabel(i, len(visible))] = l
	}
	v.state, v.hint = hinting, ""
}

// hintLabel gives one-letter labels when they're enough, two otherwise.
func hintLabel(i, n int) string {
	k := len(hintKeys)
	if n <= k {
		return string(hintKeys[i])
	}
	return string(hintKeys[i/k%k]) + string(hintKeys[i%k])
}

func (v *viewer) hintKey(k string) {
	if k == "esc" || k == "\x03" {
		v.state = reading
		return
	}
	v.hint += k
	if l, ok := v.hints[v.hint]; ok {
		v.state = reading
		v.sel = l
		v.follow(l)
		return
	}
	for label := range v.hints {
		if strings.HasPrefix(label, v.hint) {
			return
		}
	}
	v.state = reading
}

// ---- Search ----

func (v *viewer) searchKey(k string) {
	switch k {
	case "esc", "\x03":
		v.state = reading
	case "\r":
		v.state = reading
		if v.typing == "" {
			return
		}
		v.query = v.typing
		v.search(v.query, true)
	case "\x7f", "\b":
		if r := []rune(v.typing); len(r) > 0 {
			v.typing = string(r[:len(r)-1])
		} else {
			v.state = reading
		}
	case "\x15":
		v.typing = ""
	default:
		if len(k) > 0 && k[0] >= 0x20 && !strings.HasPrefix(k, "\x1b") && len([]rune(k)) == 1 {
			v.typing += k
		}
	}
}

// search finds query in the rendered text. Lowercase queries ignore case.
func (v *viewer) search(query string, jump bool) {
	pat := regexp.QuoteMeta(query)
	if strings.ToLower(query) == query {
		pat = "(?i)" + pat
	}
	re, err := regexp.Compile(pat)
	if err != nil {
		return
	}
	v.matches = nil
	for i, l := range v.doc.Lines {
		for _, m := range re.FindAllStringIndex(l.Text(), -1) {
			v.matches = append(v.matches, match{i, m[0], m[1]})
		}
	}
	if !jump {
		v.cur = min(v.cur, max(len(v.matches)-1, 0))
		return
	}
	if len(v.matches) == 0 {
		v.setFlash("not found: " + query)
		return
	}
	v.cur = 0
	for i, m := range v.matches {
		if m.line >= v.top {
			v.cur = i
			break
		}
	}
	v.show(v.matches[v.cur].line)
}

func (v *viewer) nextMatch(dir int) {
	if len(v.matches) == 0 {
		if v.query != "" {
			v.setFlash("not found: " + v.query)
		}
		return
	}
	v.cur = (v.cur + dir + len(v.matches)) % len(v.matches)
	v.show(v.matches[v.cur].line)
}

// ---- Outline ----

func (v *viewer) openOutline() {
	if len(v.doc.Headings) == 0 {
		v.setFlash("no headings")
		return
	}
	// Select the heading for the current position.
	v.osel = 0
	for i, h := range v.doc.Headings {
		if h.Line <= v.top+1 {
			v.osel = i
		}
	}
	v.state = outlining
}

func (v *viewer) outlineKey(k string) {
	n := len(v.doc.Headings)
	switch k {
	case "esc", "o", "q", "\x03":
		v.state = reading
	case "j", "down", "\t":
		v.osel = min(v.osel+1, n-1)
	case "k", "up", "shift-tab":
		v.osel = max(v.osel-1, 0)
	case "g", "home":
		v.osel = 0
	case "G", "end":
		v.osel = n - 1
	case "\r", " ":
		v.state = reading
		v.back = append(v.back, place{v.path, v.doc.SrcForLine(v.top)})
		v.fwd = nil
		v.top = v.doc.Headings[v.osel].Line
		v.clamp()
		if v.mode == sourceMode {
			v.ed.row = v.ed.rowAt(v.doc.SrcForLine(v.top))
			v.ed.top, v.ed.col = max(v.ed.row-1, 0), 0
		}
	}
}

// outlineTop is the first heading shown, so the selection stays on screen.
func (v *viewer) outlineTop() int {
	rows := v.rows() - 2
	return min(max(v.osel-rows/2, 0), max(len(v.doc.Headings)-rows, 0))
}

// ---- Drawing ----

func (v *viewer) draw() {
	v.requestNearby()
	var b strings.Builder
	b.WriteString("\x1b[?2026h\x1b[H")
	bar, hits := v.bar()
	v.hits = hits
	for _, line := range bar {
		b.WriteString("\x1b[2K" + line + "\r\n")
	}
	rows := v.rows()
	crow, ccol := -1, -1
	switch {
	case v.state == outlining:
		v.drawOutline(&b, rows)
	case v.state == helping:
		v.drawHelp(&b, rows)
	case v.mode == sourceMode:
		crow, ccol = v.drawSource(&b, rows, len(bar))
	default:
		over := v.overlays()
		for r := 0; r < rows; r++ {
			b.WriteString("\x1b[2K")
			b.WriteString(v.doc.FormatWith(v.top+r, true, over[v.top+r]))
			b.WriteString("\r\n")
		}
	}
	b.WriteString("\x1b[2K\r\n\x1b[2K")
	b.WriteString(v.status())
	if crow >= 0 && v.state == reading {
		// A bar cursor while typing, a block otherwise.
		shape := "\x1b[2 q"
		if v.ed.insert {
			shape = "\x1b[6 q"
		}
		b.WriteString(fmt.Sprintf("\x1b[%d;%dH%s\x1b[?25h", crow+1, ccol+1, shape))
	} else {
		b.WriteString("\x1b[?25l")
	}
	b.WriteString("\x1b[?2026l")
	v.write(b.String())
}

// overlays collects search matches, the selected link and link hints by line.
func (v *viewer) overlays() map[int][]doc.Overlay {
	out := map[int][]doc.Overlay{}
	inView := func(l int) bool { return l >= v.top && l < v.top+v.rows() }
	if v.state == hinting {
		first := map[int]bool{}
		label := map[int]string{}
		for l, link := range v.hints {
			label[link] = l
		}
		for _, r := range v.doc.LinkRuns() {
			if !inView(r.Line) || first[r.Link] {
				continue
			}
			if lab, ok := label[r.Link]; ok && strings.HasPrefix(lab, v.hint) {
				first[r.Link] = true
				out[r.Line] = append(out[r.Line], doc.Overlay{Start: r.Start, End: r.End, Label: lab,
					LabelStyle: doc.Style{FG: doc.Black, BG: doc.Yellow, Bold: true, Link: -1}})
			}
		}
		return out
	}
	if v.sel >= 0 {
		for _, r := range v.doc.LinkRuns() {
			if r.Link == v.sel && inView(r.Line) {
				out[r.Line] = append(out[r.Line], doc.Overlay{Start: r.Start, End: r.End, Restyle: func(s doc.Style) doc.Style {
					s.FG, s.BG, s.Dim = doc.Black, doc.Blue, false
					return s
				}})
			}
		}
	}
	for i, m := range v.matches {
		if !inView(m.line) {
			continue
		}
		current := i == v.cur
		out[m.line] = append(out[m.line], doc.Overlay{Start: m.start, End: m.end, Restyle: func(s doc.Style) doc.Style {
			s.FG, s.Dim = doc.Black, false
			s.BG = doc.Yellow
			if current {
				s.BG, s.Bold = doc.BrightYellow, true
			}
			return s
		}})
	}
	return out
}

func (v *viewer) drawOutline(b *strings.Builder, rows int) {
	margin := strings.Repeat(" ", v.doc.Margin)
	b.WriteString("\x1b[2K" + margin + "\x1b[1mOutline\x1b[0m\x1b[90m  ↑↓ move · Enter go · Esc close\x1b[0m\r\n")
	b.WriteString("\x1b[2K\r\n")
	hs := v.doc.Headings
	top := v.outlineTop()
	for r := 0; r < rows-2; r++ {
		b.WriteString("\x1b[2K")
		i := top + r
		if i < len(hs) {
			h := hs[i]
			indent := strings.Repeat("  ", max(h.Level-1, 0))
			text := runewidth.Truncate(indent+doc.Safe(h.Text), max(v.doc.Width-2, 4), "…")
			if i == v.osel {
				b.WriteString(margin + "\x1b[1;34m› " + text + "\x1b[0m")
			} else if h.Level <= 2 {
				b.WriteString(margin + "  " + text)
			} else {
				b.WriteString(margin + "\x1b[90m  " + text + "\x1b[0m")
			}
		}
		b.WriteString("\r\n")
	}
}

func (v *viewer) status() string {
	name := filepath.Base(v.path)
	left := name
	if v.rich {
		// The bar and Rex's header already show the file; show the section.
		left = ""
		for _, h := range v.doc.Headings {
			if h.Line <= v.top+1 {
				left = "§ " + h.Text
			}
		}
	}
	pos := "All"
	if v.maxTop() > 0 {
		switch v.top {
		case 0:
			pos = "Top"
		case v.maxTop():
			pos = "End"
		default:
			pos = fmt.Sprintf("%d%%", v.top*100/v.maxTop())
		}
	}
	right := "View · " + pos
	if v.mode == sourceMode && v.state == reading {
		left, right = v.sourceStatus()
		if v.flash != "" {
			right = v.flash + " · " + right
		}
	}
	switch {
	case v.mode == sourceMode && v.state == reading:
	case v.state == searching:
		left = "/" + v.typing + "▏"
		right = "Enter search · Esc cancel"
	case v.state == hinting:
		left = "follow: " + v.hint
		right = "type a letter · Esc cancel"
	case v.state == outlining:
		right = "Outline"
	case v.state == helping:
		right = "Help"
	case v.flash != "":
		right = v.flash + " · " + right
	case v.sel >= 0:
		left = "→ " + v.doc.Links[v.sel].Dest
		right = "Enter open · " + right
	case len(v.matches) > 0:
		right = fmt.Sprintf("%d/%d %q · n/N · ", v.cur+1, len(v.matches), v.query) + right
	case v.doc.HasFrontmatter && !v.showFM:
		right = "m frontmatter · " + right
	}
	if !v.rich && len(v.back) > 0 && v.state == reading && v.flash == "" && v.sel < 0 {
		left = "‹ " + left
	}
	margin := v.doc.Margin
	room := v.width - margin*2 - runewidth.StringWidth(right) - 2
	if room < 8 {
		margin = 0
		room = v.width - runewidth.StringWidth(right) - 2
	}
	if room < 8 {
		right = runewidth.Truncate(right, max(v.width/2, 4), "…")
		room = v.width - runewidth.StringWidth(right) - 2
	}
	left, right = doc.Safe(left), doc.Safe(right)
	left = runewidth.Truncate(left, max(room, 0), "…")
	gap := v.width - margin*2 - runewidth.StringWidth(left) - runewidth.StringWidth(right)
	if v.scold && v.flash != "" {
		// Bad Rexy is said in red, so it's noticed.
		return "\x1b[90m" + strings.Repeat(" ", margin) + left + strings.Repeat(" ", max(gap, 1)) + "\x1b[0m\x1b[1;31m" + right + "\x1b[0m"
	}
	return "\x1b[90m" + strings.Repeat(" ", margin) + left + strings.Repeat(" ", max(gap, 1)) + right + "\x1b[0m"
}
