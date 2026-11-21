package doc

import (
	"strings"

	"github.com/mattn/go-runewidth"
	"github.com/yuin/goldmark/ast"
	east "github.com/yuin/goldmark/extension/ast"
)

func runewidthTruncate(s string, w int) string {
	if w <= 0 {
		return ""
	}
	return runewidth.Truncate(s, w, "")
}

type cell struct {
	spans []Span
	width int
	// img, when set, is drawn in the cell instead of spans.
	img *cellImage
}

// table draws a GFM table with box-drawing borders. Columns that don't fit
// are wrapped; if the table still doesn't fit, each row becomes a card.
func (l *layout) table(n *east.Table, first, rest []Span) {
	var rows [][]cell
	var header []cell
	for r := n.FirstChild(); r != nil; r = r.NextSibling() {
		var row []cell
		for c := r.FirstChild(); c != nil; c = c.NextSibling() {
			st := Plain
			if _, ok := r.(*east.TableHeader); ok {
				st.Bold = true
			}
			if pic, img, ok := l.cellPicture(c); ok {
				link := l.addLink(pic.dest, pic.title)
				l.doc.Links[link].Image = true
				chip := []Span{l.chip(pic, st, link)}
				if img == nil {
					row = append(row, cell{spans: chip, width: spansWidth(chip)})
				} else {
					img.link = link
					row = append(row, cell{spans: chip, width: img.natural, img: img})
				}
				continue
			}
			spans := l.inlines(c, st, -1)
			row = append(row, cell{spans: spans, width: spansWidth(spans)})
		}
		if _, ok := r.(*east.TableHeader); ok {
			header = row
		} else {
			rows = append(rows, row)
		}
	}
	cols := len(n.Alignments)
	if cols == 0 {
		return
	}
	// Natural width of each column.
	natural := make([]int, cols)
	for _, row := range append([][]cell{header}, rows...) {
		for i := 0; i < cols && i < len(row); i++ {
			natural[i] = max(natural[i], row[i].width)
		}
	}
	avail := l.avail(rest)
	border := 3*cols + 1
	widths := fitColumns(natural, avail-border)
	if widths == nil {
		l.cards(header, rows, first, rest, l.srcOf(n))
		return
	}

	bs := Plain.With(BrightBlack)
	rule := func(left, mid, right string) Line {
		var b strings.Builder
		b.WriteString(left)
		for i, w := range widths {
			b.WriteString(strings.Repeat("─", w+2))
			if i < cols-1 {
				b.WriteString(mid)
			}
		}
		b.WriteString(right)
		return Line{Spans: append(clone(rest), Span{Text: b.String(), Style: bs, Src: -1}), Src: -1}
	}
	top := rule("┌", "┬", "┐")
	top.Spans = append(clone(first), top.Spans[len(rest):]...)
	l.emit(top)
	l.row(header, widths, n.Alignments, rest, bs)
	l.emit(rule("├", "┼", "┤"))
	for _, row := range rows {
		l.row(row, widths, n.Alignments, rest, bs)
	}
	l.emit(rule("└", "┴", "┘"))
}

// fitColumns shrinks columns to fit total, widest first. It returns nil when
// the columns would get too narrow to read.
func fitColumns(natural []int, total int) []int {
	widths := append([]int(nil), natural...)
	sum := 0
	for _, w := range widths {
		sum += w
	}
	const minCol = 8
	for sum > total {
		widest := 0
		for i, w := range widths {
			if w > widths[widest] {
				widest = i
			}
		}
		if widths[widest] <= minCol {
			return nil
		}
		widths[widest]--
		sum--
	}
	for i := range widths {
		widths[i] = max(widths[i], 1)
	}
	return widths
}

func (l *layout) row(row []cell, widths []int, aligns []east.Alignment, prefix []Span, bs Style) {
	// Wrap each cell to its column, then draw the row line by line.
	wrapped := make([][]Line, len(widths))
	height := 1
	for i := range widths {
		var spans []Span
		if i < len(row) {
			spans = row[i].spans
		}
		if i < len(row) && row[i].img != nil {
			wrapped[i] = l.cellImageLines(row[i].img, widths[i], len(l.doc.Lines))
		} else {
			wrapped[i] = wrap(spans, widths[i], nil, nil, -1)
		}
		height = max(height, len(wrapped[i]))
	}
	for h := 0; h < height; h++ {
		spans := append(clone(prefix), Span{Text: "│", Style: bs, Src: -1})
		src := -1
		for i, w := range widths {
			var content []Span
			if h < len(wrapped[i]) {
				content = wrapped[i][h].Spans
				if src < 0 && wrapped[i][h].Src >= 0 {
					src = wrapped[i][h].Src
				}
			}
			pad := w - spansWidth(content)
			left, right := 0, pad
			switch aligns[i] {
			case east.AlignRight:
				left, right = pad, 0
			case east.AlignCenter:
				left, right = pad/2, pad-pad/2
			}
			spans = append(spans, Span{Text: " " + spaces(left), Style: Plain, Src: -1})
			spans = append(spans, content...)
			spans = append(spans, Span{Text: spaces(right) + " ", Style: Plain, Src: -1}, Span{Text: "│", Style: bs, Src: -1})
		}
		l.emit(Line{Spans: spans, Src: src})
	}
}

// cards shows each row as a small block of "Header: value" lines, for panes
// too narrow for the table.
func (l *layout) cards(header []cell, rows [][]cell, first, rest []Span, src int) {
	bs := Plain.With(BrightBlack)
	sep := Span{Text: strings.Repeat("─", min(l.avail(rest), 24)), Style: bs, Src: -1}
	p := first
	for ri, row := range rows {
		if ri > 0 {
			l.emit(Line{Spans: append(clone(rest), sep), Src: -1})
		}
		for i, c := range row {
			label := "Column"
			if i < len(header) {
				label = plainText(header[i].spans)
			}
			lab := Span{Text: label + ": ", Style: Style{FG: BrightBlack, BG: Default, Bold: true, Link: -1}, Src: -1}
			f := append(clone(p), lab)
			r := append(clone(rest), Span{Text: "  ", Style: Plain, Src: -1})
			l.emit(wrap(c.spans, l.width, f, r, src)...)
			p = rest
		}
	}
}

var _ ast.Node = (*east.Table)(nil)
