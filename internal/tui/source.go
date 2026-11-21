package tui

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/mattn/go-runewidth"
	"golang.org/x/term"

	"github.com/travelingflwr/rex-doc/internal/doc"
)

// The two modes.
type mode int

const (
	viewMode mode = iota
	sourceMode
)

// enterSource switches to Source mode at the place shown in View.
func (v *viewer) enterSource() {
	if v.ed == nil || !v.ed.dirty {
		v.ed = newEditor(v.src)
	}
	row := v.ed.rowAt(v.doc.SrcForLine(v.top))
	if v.top == 0 {
		row = 0
	}
	v.ed.row, v.ed.col = row, 0
	v.ed.firstNonBlank()
	v.ed.top = max(row-1, 0)
	v.mode = sourceMode
	v.sel = -1
}

// leaveSource goes back to View at the cursor's place, showing the buffer
// even when it isn't saved yet.
func (v *viewer) leaveSource() {
	v.mode = viewMode
	v.leftSource = true
	if v.ed.dirty {
		v.src = []byte(v.ed.text())
	}
	off := v.ed.offsetOf(v.ed.top)
	v.doc = nil
	v.relayout()
	v.top = v.doc.LineForSrc(off)
	if v.ed.top == 0 {
		v.top = 0
	}
	v.clamp()
}

func (v *viewer) dirty() bool { return v.ed != nil && v.ed.dirty }

// saveFile writes the buffer to disk.
func (v *viewer) saveFile() bool {
	text := []byte(v.ed.text())
	if err := os.WriteFile(v.path, text, 0o644); err != nil {
		v.setFlash("can't save: " + err.Error())
		return false
	}
	v.src = text
	if st, err := os.Stat(v.path); err == nil {
		v.mod, v.size = st.ModTime(), st.Size()
	}
	v.ed.dirty = false
	v.setFlash("saved")
	return true
}

// sourceKey handles a key in Source mode and reports whether to quit.
func (v *viewer) sourceKey(k string) bool {
	v.ed.half = max(v.rows()/2, 1)
	r := v.ed.key(k)
	if v.ed.scolded {
		v.ed.scolded = false
		v.setFlash(doc.RexMark + " Bad Rexy! One space is enough")
		v.scold = true
	}
	// Unsaved means different from the file, so undoing back to it counts.
	if v.ed.dirty && !v.ed.insert {
		v.ed.dirty = v.ed.text() != string(v.src)
	}
	switch r {
	case toView:
		v.leaveSource()
	case write:
		v.saveFile()
	case writeQuit:
		return v.saveFile()
	case quitIt, forceQuit:
		return true
	case notSaved:
		v.setFlash("unsaved changes: :w to save, :q! to discard")
	case unknownCommand:
		v.setFlash("not a command: :" + v.ed.command)
	case neovim:
		v.openNeovim(v.ed.row + 1)
	}
	return false
}

// openNeovim opens the file in Neovim (or $VISUAL / $EDITOR) at line.
// In Rex it opens in a split beside rdxt, and rdxt reloads when the file
// changes. Elsewhere rdxt steps aside until the editor exits.
func (v *viewer) openNeovim(line int) {
	if v.dirty() {
		v.setFlash("unsaved changes: :w first")
		return
	}
	editor := os.Getenv("VISUAL")
	if editor == "" {
		editor = os.Getenv("EDITOR")
	}
	if editor == "" {
		if _, err := exec.LookPath("nvim"); err == nil {
			editor = "nvim"
		} else {
			editor = "vi"
		}
	}
	args := append(strings.Fields(editor), "+"+strconv.Itoa(line), v.path)
	if v.rich {
		cmd := exec.Command("rex", append([]string{"split", "--"}, args...)...)
		if cmd.Run() == nil {
			v.setFlash("editing beside · rdxt reloads on save")
			return
		}
	}
	// Step aside: normal screen, cooked terminal, run the editor, come back.
	v.write("\x1b[?1006l\x1b[?1000l\x1b[?25h\x1b[?1049l")
	term.Restore(v.inFd, v.oldState)
	cmd := exec.Command(args[0], args[1:]...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	if tty, err := os.OpenFile("/dev/tty", os.O_RDWR, 0); err == nil {
		cmd.Stdin, cmd.Stdout = tty, tty
		defer tty.Close()
	}
	err := cmd.Run()
	if st, e := term.MakeRaw(v.inFd); e == nil {
		v.oldState = st
	}
	v.write("\x1b[?1049h\x1b[?25l\x1b[?1000h\x1b[?1006h")
	if err != nil {
		v.setFlash("editor: " + err.Error())
	}
	if v.load() == nil {
		if v.ed != nil {
			v.ed.set(string(v.src))
		}
		v.relayout()
	}
}

// ---- Ticking tasks in View mode ----

// toggleTask ticks or unticks the task on doc line i and saves the file.
func (v *viewer) toggleTask(i int) {
	if v.dirty() {
		v.setFlash("unsaved Source changes: save them first")
		return
	}
	l := v.doc.Lines[i]
	// The checkbox is the first [ ] or [x] on the task's source line.
	pos := min(l.Task, len(v.src))
	start := bytes.LastIndexByte(v.src[:pos], '\n') + 1
	end := len(v.src)
	if n := bytes.IndexByte(v.src[pos:], '\n'); n >= 0 {
		end = pos + n
	}
	line := v.src[start:end]
	j := bytes.IndexByte(line, '[')
	if j < 0 || j+2 >= len(line) || line[j+2] != ']' {
		return
	}
	at := start + j + 1
	prev := append([]byte(nil), v.src...)
	next := append([]byte(nil), v.src...)
	if next[at] == ' ' {
		next[at] = 'x'
	} else {
		next[at] = ' '
	}
	if err := os.WriteFile(v.path, next, 0o644); err != nil {
		v.setFlash("can't save: " + err.Error())
		return
	}
	v.viewUndo = append(v.viewUndo, prev)
	v.src = next
	if st, err := os.Stat(v.path); err == nil {
		v.mod, v.size = st.ModTime(), st.Size()
	}
	v.ed = nil
	v.relayout()
	v.setFlash("saved · u to undo")
}

// undoView undoes the last edit made from View mode.
func (v *viewer) undoView() {
	if len(v.viewUndo) == 0 || v.dirty() {
		v.setFlash("nothing to undo")
		return
	}
	prev := v.viewUndo[len(v.viewUndo)-1]
	v.viewUndo = v.viewUndo[:len(v.viewUndo)-1]
	if err := os.WriteFile(v.path, prev, 0o644); err != nil {
		v.setFlash("can't save: " + err.Error())
		return
	}
	v.src = prev
	if st, err := os.Stat(v.path); err == nil {
		v.mod, v.size = st.ModTime(), st.Size()
	}
	v.ed = nil
	v.relayout()
	v.setFlash("undone")
}

// ---- Drawing Source mode ----

// seg is one screen row of a buffer line, after soft wrapping.
type seg struct {
	row   int // buffer line
	start int // first rune
	spans []doc.Span
}

// sourceLayout wraps buffer lines from e.top into screen rows.
func (v *viewer) sourceLayout(width, rows int) []seg {
	e := v.ed
	text := e.text()
	if text != v.hlText {
		lang := "markdown"
		if o := doc.OptionsFor(v.path); o.Code != "" {
			lang = o.Code
		} else if o.Dok || o.Text || !isDocument(v.path) {
			lang = ""
		}
		v.hl = doc.Highlight(strings.TrimSuffix(text, "\n"), lang)
		v.hlText = text
	}
	var out []seg
	for r := e.top; r < len(e.lines) && len(out) < rows+1; r++ {
		var spans []doc.Span
		if r < len(v.hl) && spansText(v.hl[r]) == string(e.lines[r]) {
			spans = v.hl[r]
		} else {
			spans = []doc.Span{{Text: string(e.lines[r]), Style: doc.Plain, Src: -1}}
		}
		out = append(out, wrapSpans(r, spans, width)...)
	}
	return out
}

func spansText(s []doc.Span) string {
	var b strings.Builder
	for _, x := range s {
		b.WriteString(x.Text)
	}
	return b.String()
}

// wrapSpans breaks a line's spans into rows of at most width cells, by
// character, so the cursor maps exactly.
func wrapSpans(row int, spans []doc.Span, width int) []seg {
	segs := []seg{{row: row}}
	w, idx := 0, 0
	for _, s := range spans {
		var cur strings.Builder
		flush := func() {
			if cur.Len() > 0 {
				n := len(segs) - 1
				segs[n].spans = append(segs[n].spans, doc.Span{Text: cur.String(), Style: s.Style, Src: -1})
				cur.Reset()
			}
		}
		for _, r := range strings.ReplaceAll(s.Text, "\t", "    ") {
			rw := runewidth.RuneWidth(r)
			if w+rw > width && w > 0 {
				flush()
				segs = append(segs, seg{row: row, start: idx})
				w = 0
			}
			cur.WriteRune(r)
			w += rw
			idx++
		}
		flush()
	}
	return segs
}

func (v *viewer) gutterWidth() int { return len(strconv.Itoa(len(v.ed.lines))) + 2 }

// textCol is the screen column where buffer text starts: one space, the
// line number padded to gw-2, then two spaces.
func textCol(gw int) int { return 1 + (gw - 2) + 2 }

// drawSource draws the buffer and returns where the cursor goes (0-based
// row and column on screen), or -1 when it's off screen.
func (v *viewer) drawSource(b *strings.Builder, rows, top int) (int, int) {
	e := v.ed
	gw := v.gutterWidth()
	width := max(v.width-gw-2, 8)
	// Keep the cursor on screen.
	if e.row < e.top {
		e.top = e.row
	}
	for {
		segs := v.sourceLayout(width, rows)
		crow, ccol := cursorIn(segs, e, rows)
		if crow >= 0 || e.top >= e.row {
			for i := 0; i < rows; i++ {
				b.WriteString("\x1b[2K")
				if i < len(segs) {
					s := segs[i]
					num := ""
					if s.start == 0 {
						num = strconv.Itoa(s.row + 1)
					}
					style := "\x1b[90m"
					if s.row == e.row {
						style = "\x1b[1m"
					}
					b.WriteString(" " + style + fmt.Sprintf("%*s", gw-2, num) + "\x1b[0m  ")
					for _, sp := range s.spans {
						sgr := sp.Style.SGR()
						b.WriteString(sgr + doc.Safe(sp.Text))
						if sgr != "" {
							b.WriteString("\x1b[0m")
						}
					}
				} else if e.top+i >= len(e.lines) && i > 0 {
					b.WriteString(" \x1b[90m" + strings.Repeat(" ", gw-3) + "~\x1b[0m")
				}
				b.WriteString("\r\n")
			}
			if crow < 0 {
				return -1, -1
			}
			return top + crow, textCol(gw) + ccol
		}
		e.top++
	}
}

// cursorIn finds the cursor's screen row and column among segs.
func cursorIn(segs []seg, e *editor, rows int) (int, int) {
	for i, s := range segs {
		if i >= rows || s.row != e.row {
			continue
		}
		end := s.start + runeCount(s.spans)
		last := i+1 >= len(segs) || segs[i+1].row != e.row
		if e.col >= s.start && (e.col < end || last) {
			l := e.lines[e.row]
			return i, runewidth.StringWidth(strings.ReplaceAll(string(l[s.start:min(e.col, len(l))]), "\t", "    "))
		}
	}
	return -1, -1
}

func runeCount(spans []doc.Span) int {
	n := 0
	for _, s := range spans {
		n += len([]rune(s.Text))
	}
	return n
}

// sourceClick puts the cursor where the screen was tapped.
func (v *viewer) sourceClick(x, y int) {
	gw := v.gutterWidth()
	width := max(v.width-gw-2, 8)
	segs := v.sourceLayout(width, v.rows())
	if y < 0 || y >= len(segs) {
		return
	}
	s := segs[y]
	col := s.start
	w := 0
	target := x - textCol(gw)
	for _, r := range []rune(spansText(s.spans)) {
		rw := runewidth.RuneWidth(r)
		if w+rw > target {
			break
		}
		w += rw
		col++
	}
	v.ed.row, v.ed.col = s.row, col
	v.ed.clampCursor()
	v.ed.want = dispCol(v.ed.cur(), v.ed.col)
}

func (v *viewer) sourceStatus() (string, string) {
	e := v.ed
	if e.commanding {
		return ":" + e.command + "▏", "Enter run · Esc cancel"
	}
	left := fmt.Sprintf("Ln %d, Col %d", e.row+1, e.col+1)
	var right string
	if e.insert {
		right = "Insert · Esc to stop"
	} else {
		right = "Source · i insert · :w save · Esc view"
	}
	if e.dirty {
		left += " · ● unsaved"
	}
	return left, right
}
