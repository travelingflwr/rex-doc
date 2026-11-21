package doc

import "strings"

// tabStop is how far apart tab stops are in plain text.
const tabStop = 8

// renderText lays plain text (.txt) out line by line. A line that fits is
// shown exactly as written, so columns and ASCII drawings stay aligned. A
// longer line wraps, and its continuation lines keep its indent.
func renderText(src []byte, d *Doc) {
	l := &layout{src: src, width: d.Width, doc: d, slugs: map[string]int{}}
	off := 0
	for _, raw := range strings.SplitAfter(string(src), "\n") {
		start := off
		off += len(raw)
		line := strings.TrimRight(raw, "\r\n")
		if strings.TrimSpace(line) == "" {
			if raw != "" {
				l.emit(Line{Src: start})
			}
			continue
		}
		spans, width := textSpans(line, start)
		if width <= d.Width {
			l.emit(Line{Spans: spans, Src: start})
			continue
		}
		body := strings.TrimLeft(line, " \t")
		lead := line[:len(line)-len(body)]
		indent := min(columns(lead), d.Width/2)
		pad := []Span{{Text: spaces(indent), Style: Plain, Src: -1}}
		l.emit(wrap([]Span{{Text: body, Style: Plain, Src: start + len(lead)}}, d.Width, pad, pad, start)...)
	}
	for len(d.Lines) > 0 && isBlank(d.Lines[len(d.Lines)-1]) {
		d.Lines = d.Lines[:len(d.Lines)-1]
	}
}

// textSpans is a line as spans, with tabs drawn as spaces to the next tab
// stop. The spaces aren't source text, so they have no source offset.
func textSpans(line string, src int) ([]Span, int) {
	var spans []Span
	col := 0
	for {
		i := strings.IndexByte(line, '\t')
		if i < 0 {
			break
		}
		if i > 0 {
			spans = append(spans, Span{Text: line[:i], Style: Plain, Src: src})
			col += textWidth(line[:i])
		}
		n := tabStop - col%tabStop
		spans = append(spans, Span{Text: spaces(n), Style: Plain, Src: -1})
		col += n
		line = line[i+1:]
		src += i + 1
	}
	if line != "" {
		spans = append(spans, Span{Text: line, Style: Plain, Src: src})
		col += textWidth(line)
	}
	return spans, col
}

// columns is how wide leading whitespace is, with tabs to the next stop.
func columns(lead string) int {
	col := 0
	for _, r := range lead {
		if r == '\t' {
			col += tabStop - col%tabStop
		} else {
			col++
		}
	}
	return col
}
