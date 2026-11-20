package doc

import (
	"fmt"
	"hash/fnv"
	"image"
	_ "image/gif" // registered for DecodeConfig
	_ "image/jpeg"
	_ "image/png"
	"os"
	"path/filepath"
	"strings"

	_ "golang.org/x/image/webp"
)

// Figure is an image shown as a figure. Its placeholder cells sit on the
// lines from Line to Line+Rows-1; the terminal draws the image there once
// rdxt has sent it with ID.
type Figure struct {
	ID         uint32
	Path       string
	Cols, Rows int
	Line       int
	Width      int // the image's own size in pixels
	Height     int
}

// Placeholder is kitty's Unicode placeholder character. Each image row
// starts with it plus a row and a column diacritic; the rest of the row
// is the bare character.
const Placeholder = "\U0010EEEE"

// rowDiacritics are the first entries of kitty's row/column diacritics, so
// figures can be at most this many rows tall.
var rowDiacritics = []rune{0x0305, 0x030D, 0x030E, 0x0310, 0x0312, 0x033D, 0x033E, 0x033F,
	0x0346, 0x034A, 0x034B, 0x034C, 0x0350, 0x0351, 0x0352, 0x0357, 0x035B, 0x0363, 0x0364,
	0x0365, 0x0366, 0x0367, 0x0368, 0x0369, 0x036A, 0x036B, 0x036C, 0x036D, 0x036E, 0x036F}

// MaxFigureRows is the tallest a figure can be.
var MaxFigureRows = len(rowDiacritics)

// PlaceholderRow is one row of placeholder cells for an image cols wide.
func PlaceholderRow(r, cols int) string {
	return Placeholder + string(rowDiacritics[r]) + string(rowDiacritics[0]) + strings.Repeat(Placeholder, cols-1)
}

// figure lays out an image on its own line: a frame, the image's cells and
// a numbered caption. Remote, missing and unreadable images get a card.
func (l *layout) figure(p picture, first, rest []Span) {
	l.figures++
	dest := p.dest
	label := p.alt
	if p.title != "" {
		label = p.title
	}
	if label == "" {
		label = filepath.Base(dest)
	}
	caption := fmt.Sprintf("Figure %d · %s", l.figures, label)
	src := p.src
	link := l.addLink(dest, p.title)
	l.doc.Links[link].Image = true

	if host, remote := isRemote(dest); remote {
		l.card(first, rest, label, "on "+host+" · tap to open in the browser", caption, link, src)
		return
	}
	if _, err := os.Stat(l.localPath(dest)); err != nil {
		l.card(first, rest, "image not found", dest, caption, -1, src)
		return
	}
	path, cfg, ok := l.readable(dest)
	if !ok {
		l.card(first, rest, label, dest+" · can't show this kind of image yet", caption, link, src)
		return
	}
	if !l.opts.Images {
		l.card(first, rest, label, dest, caption, link, src)
		return
	}
	l.placeFigure(path, cfg, p.width, caption, link, src, first, rest)
}

// placeFigure lays out a readable image as a figure: frame, cells, caption.
func (l *layout) placeFigure(path string, cfg image.Config, want int, caption string, link, src int, first, rest []Span) {
	avail := l.avail(rest) - 4
	limit := avail
	if want > 0 { // an <img width="…">
		limit = min(avail, max(want/cssCell, 4))
	}
	cols, rows := l.fitCells(cfg, limit, l.maxFigureRows())
	id := figureID(path, cols, rows)
	indent := Span{Text: spaces((avail - cols) / 2), Style: Plain, Src: -1}
	frame := Plain.With(BrightBlack)

	top := append(clone(first), indent, Span{Text: "╭" + strings.Repeat("─", cols+2) + "╮", Style: frame, Src: src})
	l.emit(Line{Spans: top, Src: src})
	start := len(l.doc.Lines)
	for r := 0; r < rows; r++ {
		cells := PlaceholderRow(r, cols)
		st := Style{FG: Default, BG: Default, Link: link, Image: id}
		spans := append(clone(rest), indent,
			Span{Text: "│ ", Style: frame, Src: -1},
			Span{Text: cells, Style: st, Src: -1},
			Span{Text: " │", Style: frame, Src: -1})
		l.emit(Line{Spans: spans, Src: src})
	}
	bottom := append(clone(rest), indent, Span{Text: "╰" + strings.Repeat("─", cols+2) + "╯", Style: frame, Src: -1})
	l.emit(Line{Spans: bottom, Src: -1})
	l.caption(rest, indent, caption, src)
	l.doc.Figures = append(l.doc.Figures, Figure{ID: id, Path: path, Cols: cols, Rows: rows, Line: start,
		Width: cfg.Width, Height: cfg.Height})
}

// card stands in for an image that isn't drawn: a framed box with a title
// and a detail line, then the caption.
func (l *layout) card(first, rest []Span, title, detail, caption string, link, src int) {
	frame := Plain.With(BrightBlack)
	inner := min(max(textWidth(title), textWidth(detail))+2, l.avail(rest)-2)
	ts := Style{FG: Default, BG: Default, Bold: true, Link: link}
	ds := Style{FG: BrightBlack, BG: Default, Link: link}
	row := func(text string, st Style, p []Span) {
		text = truncate(text, inner-1)
		l.emit(Line{Spans: append(clone(p), Span{Text: "│ ", Style: frame, Src: -1},
			Span{Text: text, Style: st, Src: src},
			Span{Text: spaces(inner-1-textWidth(text)) + "│", Style: frame, Src: -1}), Src: src})
	}
	l.emit(Line{Spans: append(clone(first), Span{Text: "╭" + strings.Repeat("─", inner) + "╮", Style: frame, Src: src}), Src: src})
	row(title, ts, rest)
	row(detail, ds, rest)
	l.emit(Line{Spans: append(clone(rest), Span{Text: "╰" + strings.Repeat("─", inner) + "╯", Style: frame, Src: -1}), Src: -1})
	l.caption(rest, Span{Style: Plain, Src: -1}, caption, src)
}

func (l *layout) caption(rest []Span, indent Span, text string, src int) {
	st := Style{FG: BrightBlack, BG: Default, Italic: true, Link: -1}
	p := append(clone(rest), indent, Span{Text: "  ", Style: Plain, Src: -1})
	l.emit(wrap([]Span{{Text: text, Style: st, Src: src}}, l.width, p, p, src)...)
}

// figureID gives a figure a stable 24-bit image ID (never 0) from its file
// and size, since the ID is also the placeholder cells' color.
func figureID(path string, cols, rows int) uint32 {
	h := fnv.New32a()
	fmt.Fprintf(h, "%s|%d|%d", path, cols, rows)
	id := h.Sum32() & 0xFFFFFF
	if id == 0 {
		id = 1
	}
	return id
}
