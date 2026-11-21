package doc

import (
	"fmt"
	"strings"
)

// renderCode lays a source file out with syntax colors and line numbers.
// For Rex's config file, Rex's check of it comes first, and the lines it
// complains about have red numbers.
func renderCode(src []byte, d *Doc, opts Options) {
	l := &layout{src: src, width: d.Width, doc: d, slugs: map[string]int{}, opts: opts}
	bad := map[int]bool{}
	if opts.Check != nil {
		l.checkCard(opts.Check)
		for _, p := range opts.Check.Problems {
			if p.Line > 0 {
				bad[p.Line] = true
			}
		}
	}
	text := string(src)
	lines := highlight(strings.TrimSuffix(text, "\n"), opts.Code)
	numW := len(fmt.Sprint(len(lines)))
	num := Plain.With(BrightBlack)
	off := 0
	for i, spans := range lines {
		start := off
		end := strings.IndexByte(text[off:], '\n')
		if end < 0 {
			off = len(text)
		} else {
			off += end + 1
		}
		spans = withSrc(spans, text[start:max(off, start)], start)
		st := num
		if bad[i+1] {
			st = Style{FG: Red, BG: Default, Bold: true, Link: -1}
		}
		gutter := []Span{{Text: fmt.Sprintf("%*d │ ", numW, i+1), Style: st, Src: -1}}
		cont := []Span{{Text: spaces(numW) + " │ ", Style: num, Src: -1}}
		spans, w := expandTabs(spans)
		if w <= d.Width-spansWidth(gutter) {
			l.emit(Line{Spans: append(gutter, spans...), Src: start})
			continue
		}
		l.emit(wrap(spans, d.Width, gutter, cont, start)...)
	}
}

// withSrc gives a line's highlighted spans their source offsets, when they
// spell out the line exactly.
func withSrc(spans []Span, line string, start int) []Span {
	var b strings.Builder
	for _, s := range spans {
		b.WriteString(s.Text)
	}
	if b.String() != strings.TrimRight(line, "\r\n") {
		return spans
	}
	out := make([]Span, len(spans))
	at := start
	for i, s := range spans {
		s.Src = at
		at += len(s.Text)
		out[i] = s
	}
	return out
}

// expandTabs draws tabs as spaces to the next tab stop, and returns the
// line's width.
func expandTabs(spans []Span) ([]Span, int) {
	var out []Span
	col := 0
	for _, s := range spans {
		for {
			i := strings.IndexByte(s.Text, '\t')
			if i < 0 {
				break
			}
			if i > 0 {
				out = append(out, Span{Text: s.Text[:i], Style: s.Style, Src: s.Src})
				col += textWidth(s.Text[:i])
			}
			n := tabStop - col%tabStop
			out = append(out, Span{Text: spaces(n), Style: Plain, Src: -1})
			col += n
			if s.Src >= 0 {
				s.Src += i + 1
			}
			s.Text = s.Text[i+1:]
		}
		if s.Text != "" {
			out = append(out, s)
			col += textWidth(s.Text)
		}
	}
	return out, col
}

// checkCard shows Rex's check of its config: a green line when Rex accepts
// it, or an alert with each problem.
func (l *layout) checkCard(c *Check) {
	errors := 0
	for _, p := range c.Problems {
		if p.Severity == "error" {
			errors++
		}
	}
	color, icon, title := Green, "✔", "Rex accepts this config"
	switch {
	case c.Err != "":
		color, icon, title = Yellow, "▲", "Couldn't check this config"
	case errors > 0:
		color, icon, title = Red, "▼", plural(errors, "error")+" in this config"
	case len(c.Problems) > 0:
		color, icon, title = Yellow, "▲", plural(len(c.Problems), "warning")+" in this config"
	}
	bar := []Span{{Text: "▎ ", Style: Plain.With(color), Src: -1}}
	head := Style{FG: color, BG: Default, Bold: true, Link: -1}
	l.emit(Line{Spans: append(bar, Span{Text: icon + " " + title, Style: head, Src: -1}), Src: -1})
	note := c.Summary
	if c.Err != "" {
		note = c.Err
	}
	if note != "" {
		l.emit(wrap([]Span{{Text: note, Style: Plain.With(BrightBlack), Src: -1}}, l.width, bar, bar, -1)...)
	}
	for _, p := range c.Problems {
		where := p.Where
		if p.Line > 0 {
			where = fmt.Sprintf("line %d", p.Line)
		}
		mark := Style{FG: Red, BG: Default, Bold: true, Link: -1}
		if p.Severity != "error" {
			mark.FG = Yellow
		}
		first := append(clone(bar), Span{Text: where + "  ", Style: mark, Src: -1})
		rest := append(clone(bar), Span{Text: spaces(textWidth(where) + 2), Style: Plain, Src: -1})
		l.emit(wrap([]Span{{Text: p.Message, Style: Plain, Src: -1}}, l.width, first, rest, -1)...)
	}
	l.emit(Line{Src: -1})
}

func plural(n int, what string) string {
	if n == 1 {
		return "1 " + what
	}
	return fmt.Sprintf("%d %ss", n, what)
}
