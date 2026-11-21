package doc

import (
	"bytes"
	"fmt"
	"strings"
	"unicode"

	"github.com/yuin/goldmark"
	meta "github.com/yuin/goldmark-meta"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	east "github.com/yuin/goldmark/extension/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
	"gopkg.in/yaml.v2"
)

// Layout widths.
const (
	// ReadingWidth is the widest the text gets, for comfortable reading.
	ReadingWidth = 88
	// CompactBelow is the terminal width under which the compact layout is used.
	CompactBelow = 60
)

var md = goldmark.New(
	goldmark.WithExtensions(extension.GFM, extension.Footnote, meta.Meta),
	goldmark.WithParserOptions(parser.WithAttribute()),
)

// Render parses Markdown and lays it out for a terminal termWidth cells wide.
func Render(src []byte, termWidth int, opts Options) *Doc {
	if termWidth < 10 {
		termWidth = 10
	}
	d := &Doc{}
	if termWidth < CompactBelow {
		d.Margin = 1
		d.Width = termWidth - 2
	} else {
		d.Width = min(termWidth-4, ReadingWidth)
		d.Margin = (termWidth - d.Width) / 2
	}
	if opts.Dok {
		renderDok(src, d)
		return d
	}
	ctx := parser.NewContext()
	root := md.Parser().Parse(text.NewReader(src), parser.WithContext(ctx))
	l := &layout{src: src, width: d.Width, doc: d, slugs: map[string]int{}, opts: opts}
	if items, err := meta.TryGetItems(ctx); err == nil && len(items) > 0 {
		d.HasFrontmatter = true
		if opts.ShowFrontmatter {
			l.frontmatter(items)
		}
		// A frontmatter title stands in for the H1 when the document has none.
		if title := frontmatterTitle(items); title != "" && !hasH1(root) {
			l.titleHeading(title)
		}
	}
	l.blocks(root, nil, nil, false)
	// Drop trailing blank lines.
	for len(d.Lines) > 0 && isBlank(d.Lines[len(d.Lines)-1]) {
		d.Lines = d.Lines[:len(d.Lines)-1]
	}
	return d
}

type layout struct {
	opts      Options
	figures   int
	src       []byte
	width     int
	doc       *Doc
	listDepth int
	slugs     map[string]int
}

func isBlank(l Line) bool {
	for _, s := range l.Spans {
		if strings.TrimSpace(s.Text) != "" {
			return false
		}
	}
	return true
}

func (l *layout) emit(lines ...Line) { l.doc.Lines = append(l.doc.Lines, lines...) }

// blank adds an empty line that keeps the prefix's bars (for quotes), without
// its bullets or trailing spaces.
func (l *layout) blank(prefix []Span) {
	var spans []Span
	for _, s := range prefix {
		t := strings.TrimRight(s.Text, " ")
		if t != "" {
			spans = append(spans, Span{Text: t, Style: s.Style, Src: -1})
		}
	}
	l.emit(Line{Spans: spans, Src: -1})
}

func (l *layout) lastBlank() bool {
	n := len(l.doc.Lines)
	return n == 0 || isBlank(l.doc.Lines[n-1])
}

// blocks lays out the children of parent. first is the prefix for the first
// line of the first child and rest the prefix for every other line. tight
// lists have no blank lines between their children.
func (l *layout) blocks(parent ast.Node, first, rest []Span, tight bool) {
	i := 0
	for c := parent.FirstChild(); c != nil; c = c.NextSibling() {
		p := rest
		if i == 0 {
			p = first
		}
		if i > 0 && !tight {
			l.blank(rest)
			if h, ok := c.(*ast.Heading); ok && h.Level <= 2 && l.listDepth == 0 {
				l.blank(rest)
			}
		}
		l.block(c, p, rest)
		i++
	}
}

// srcOf returns the source offset where n starts.
func (l *layout) srcOf(n ast.Node) int {
	if n.Type() == ast.TypeBlock {
		if lines := n.Lines(); lines != nil && lines.Len() > 0 {
			return lines.At(0).Start
		}
	}
	if t, ok := n.(*ast.Text); ok {
		return t.Segment.Start
	}
	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		if s := l.srcOf(c); s >= 0 {
			return s
		}
	}
	return -1
}

func spaces(n int) string { return strings.Repeat(" ", max(n, 0)) }

func (l *layout) avail(prefix []Span) int { return max(l.width-spansWidth(prefix), 8) }

func (l *layout) block(n ast.Node, first, rest []Span) {
	switch n := n.(type) {
	case *ast.Heading:
		l.heading(n, first, rest)
	case *ast.Paragraph, *ast.TextBlock:
		if pic, ok := l.soloPicture(n); ok {
			l.figure(pic, first, rest)
			return
		}
		spans := l.inlines(n, Plain, -1)
		l.emit(wrap(spans, l.width, first, rest, l.srcOf(n))...)
	case *ast.Blockquote:
		l.blockquote(n, first, rest)
	case *ast.List:
		l.list(n, first, rest)
	case *ast.FencedCodeBlock:
		lang := ""
		if n.Info != nil {
			lang = strings.Fields(string(n.Info.Segment.Value(l.src)) + " ")[0]
		}
		l.code(n, lang, first, rest)
	case *ast.CodeBlock:
		l.code(n, "", first, rest)
	case *ast.ThematicBreak:
		rule := strings.Repeat("─", l.avail(rest))
		l.emit(Line{Spans: append(clone(first), Span{Text: rule, Style: Plain.With(BrightBlack), Src: -1}), Src: -1})
	case *ast.HTMLBlock:
		var b strings.Builder
		for i := 0; i < n.Lines().Len(); i++ {
			seg := n.Lines().At(i)
			b.Write(seg.Value(l.src))
		}
		if n.HasClosure() {
			b.Write(n.ClosureLine.Value(l.src))
		}
		p := first
		src := l.srcOf(n)
		if pics := blockPictures(b.String(), max(src, 0)); len(pics) > 0 {
			for i, pic := range pics {
				if i > 0 {
					l.blank(rest)
				}
				l.figure(pic, p, rest)
				p = rest
			}
			return
		}
		st := Plain
		st.Dim = true
		for _, line := range strings.Split(strings.TrimRight(b.String(), "\n"), "\n") {
			l.emit(Line{Spans: append(clone(p), Span{Text: truncate(line, l.avail(p)), Style: st, Src: src}), Src: src})
			p = rest
		}
	case *east.Table:
		l.table(n, first, rest)
	case *east.FootnoteList:
		l.footnotes(n, first, rest)
	default:
		if n.HasChildren() {
			l.blocks(n, first, rest, false)
		}
	}
}

func clone(s []Span) []Span { return append([]Span(nil), s...) }

var headingStyles = [7]Style{
	1: {FG: Magenta, BG: Default, Bold: true, Link: -1},
	2: {FG: Blue, BG: Default, Bold: true, Link: -1},
	3: {FG: Cyan, BG: Default, Bold: true, Link: -1},
	4: {FG: Default, BG: Default, Bold: true, Link: -1},
	5: {FG: Default, BG: Default, Bold: true, Dim: true, Link: -1},
	6: {FG: Default, BG: Default, Italic: true, Dim: true, Link: -1},
}

func (l *layout) heading(n *ast.Heading, first, rest []Span) {
	st := headingStyles[min(max(n.Level, 1), 6)]
	spans := l.inlines(n, st, -1)
	src := l.srcOf(n)
	plain := plainText(spans)
	l.doc.Headings = append(l.doc.Headings, Heading{Level: n.Level, Text: plain, Slug: l.slug(plain), Line: len(l.doc.Lines)})
	if n.Level == 1 {
		spans = upper(spans)
	}
	lines := wrap(spans, l.width, first, rest, src)
	for i := range lines {
		lines[i].Heading = n.Level
	}
	l.emit(lines...)
	switch n.Level {
	case 1:
		l.emit(Line{Spans: append(clone(rest), Span{Text: strings.Repeat("━", l.avail(rest)), Style: Plain.With(Magenta), Src: -1}), Src: -1})
	case 2:
		l.emit(Line{Spans: append(clone(rest), Span{Text: strings.Repeat("─", l.avail(rest)), Style: Plain.With(BrightBlack), Src: -1}), Src: -1})
	}
}

func plainText(spans []Span) string {
	var b strings.Builder
	for _, s := range spans {
		if s.Style.Image != 0 { // placeholder cells of an image, not text
			continue
		}
		b.WriteString(s.Text)
	}
	return strings.TrimSpace(b.String())
}

// upper capitalizes H1 text so it stands out. Inline code keeps its case.
func upper(spans []Span) []Span {
	out := clone(spans)
	for i := range out {
		if out[i].Style.FG != Cyan {
			out[i].Text = strings.ToUpper(out[i].Text)
		}
	}
	return out
}

func hasH1(root ast.Node) bool {
	found := false
	ast.Walk(root, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if h, ok := n.(*ast.Heading); ok && entering && h.Level == 1 {
			found = true
			return ast.WalkStop, nil
		}
		return ast.WalkContinue, nil
	})
	return found
}

func frontmatterTitle(items yaml.MapSlice) string {
	for _, it := range items {
		if strings.EqualFold(fmt.Sprint(it.Key), "title") {
			return strings.TrimSpace(fmt.Sprint(it.Value))
		}
	}
	return ""
}

// titleHeading draws a frontmatter title as the document's H1.
func (l *layout) titleHeading(title string) {
	l.doc.Headings = append(l.doc.Headings, Heading{Level: 1, Text: title, Slug: l.slug(title), Line: len(l.doc.Lines)})
	lines := wrap([]Span{{Text: strings.ToUpper(title), Style: headingStyles[1], Src: 0}}, l.width, nil, nil, 0)
	for i := range lines {
		lines[i].Heading = 1
	}
	l.emit(lines...)
	l.emit(Line{Spans: []Span{{Text: strings.Repeat("━", l.width), Style: Plain.With(Magenta), Src: -1}}, Src: -1})
	l.emit(Line{Src: -1})
}

// slug makes a GitHub-style anchor for a heading.
func (l *layout) slug(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-' || r == '_':
			b.WriteRune(r)
		case r == ' ':
			b.WriteRune('-')
		}
	}
	slug := b.String()
	if n, ok := l.slugs[slug]; ok {
		l.slugs[slug] = n + 1
		return fmt.Sprintf("%s-%d", slug, n+1)
	}
	l.slugs[slug] = 0
	return slug
}

// Alerts, as GitHub writes them: > [!NOTE]
var alertKinds = map[string]struct {
	title string
	icon  string
	color Color
}{
	"NOTE":      {"Note", "●", Blue},
	"TIP":       {"Tip", "◆", Green},
	"IMPORTANT": {"Important", "■", Magenta},
	"WARNING":   {"Warning", "▲", Yellow},
	"CAUTION":   {"Caution", "▼", Red},
}

func (l *layout) blockquote(n *ast.Blockquote, first, rest []Span) {
	barStyle := Plain.With(BrightBlack)
	// Is it an alert? The first line of the first paragraph is [!KIND].
	if p, ok := n.FirstChild().(*ast.Paragraph); ok && p.Lines().Len() > 0 {
		seg := p.Lines().At(0)
		marker := strings.TrimSpace(string(seg.Value(l.src)))
		if strings.HasPrefix(marker, "[!") && strings.HasSuffix(marker, "]") {
			if kind, ok := alertKinds[strings.ToUpper(marker[2:len(marker)-1])]; ok {
				bar := Span{Text: "▎ ", Style: Plain.With(kind.color), Src: -1}
				f := append(clone(first), bar)
				r := append(clone(rest), bar)
				title := Style{FG: kind.color, BG: Default, Bold: true, Link: -1}
				l.emit(Line{Spans: append(f, Span{Text: kind.icon + " " + kind.title, Style: title, Src: seg.Start}), Src: seg.Start})
				// The rest of the first paragraph, then the other children.
				if spans := l.inlines(p, Plain, seg.Stop); len(spans) > 0 && plainText(spans) != "" {
					l.emit(wrap(spans, l.width, r, r, l.srcOf(p))...)
				}
				for c := p.NextSibling(); c != nil; c = c.NextSibling() {
					l.blank(r)
					l.block(c, r, r)
				}
				return
			}
		}
	}
	bar := Span{Text: "▎ ", Style: barStyle, Src: -1}
	l.blocks(n, append(clone(first), bar), append(clone(rest), bar), false)
}

var bullets = []string{"•", "◦", "▪"}

func (l *layout) list(n *ast.List, first, rest []Span) {
	l.listDepth++
	defer func() { l.listDepth-- }()
	num := n.Start
	if num == 0 {
		num = 1
	}
	// Ordered markers are right-aligned to the widest number.
	count := n.ChildCount()
	numW := len(fmt.Sprint(num + count - 1))
	i := 0
	for item := n.FirstChild(); item != nil; item = item.NextSibling() {
		p := rest
		if i == 0 {
			p = first
		}
		if i > 0 && !n.IsTight {
			l.blank(rest)
		}
		var marker []Span
		done := false
		bold := Style{FG: Default, BG: Default, Bold: true, Link: -1}
		switch {
		case taskBox(item) != nil:
			// Round brackets: ( ) when open, (✔) when done.
			if taskBox(item).IsChecked {
				done = true
				marker = []Span{{Text: "(", Style: bold, Src: -1}, {Text: "✔", Style: Style{FG: Green, BG: Default, Bold: true, Link: -1}, Src: -1}, {Text: ")", Style: bold, Src: -1}}
			} else {
				marker = []Span{{Text: "( )", Style: bold, Src: -1}}
			}
		case n.IsOrdered():
			marker = []Span{{Text: fmt.Sprintf("%*d.", numW, num), Style: Plain.With(BrightBlack), Src: -1}}
			num++
		default:
			marker = []Span{{Text: bullets[(l.listDepth-1)%len(bullets)], Style: Plain.With(BrightBlack), Src: -1}}
		}
		mw := spansWidth(marker) + 1
		f := append(append(clone(p), marker...), Span{Text: " ", Style: Plain, Src: -1})
		r := append(clone(rest), Span{Text: spaces(mw), Style: Plain, Src: -1})
		startLine := len(l.doc.Lines)
		l.blocks(item, f, r, n.IsTight)
		if taskBox(item) != nil && startLine < len(l.doc.Lines) {
			l.doc.Lines[startLine].Task = l.srcOf(item)
			l.doc.Lines[startLine].TaskCols = spansWidth(f)
		}
		if done {
			// A finished task's text fades back.
			for li := startLine; li < len(l.doc.Lines); li++ {
				for si := range l.doc.Lines[li].Spans {
					if l.doc.Lines[li].Spans[si].Src >= 0 {
						l.doc.Lines[li].Spans[si].Style.Dim = true
					}
				}
			}
		}
		i++
	}
}

// taskBox returns the task-list checkbox at the start of a list item, if any.
func taskBox(item ast.Node) *east.TaskCheckBox {
	c := item.FirstChild()
	if c == nil {
		return nil
	}
	if b, ok := c.FirstChild().(*east.TaskCheckBox); ok {
		return b
	}
	return nil
}

func (l *layout) code(n ast.Node, lang string, first, rest []Span) {
	var b bytes.Buffer
	lines := n.Lines()
	for i := 0; i < lines.Len(); i++ {
		seg := lines.At(i)
		b.Write(seg.Value(l.src))
	}
	src := strings.TrimRight(strings.ReplaceAll(b.String(), "\t", "    "), "\n")
	start := l.srcOf(n)
	if strings.EqualFold(lang, "mermaid") && l.diagram(src, start, first, rest) {
		return
	}
	gutter := Span{Text: "│ ", Style: Plain.With(BrightBlack), Src: -1}
	if lang != "" {
		label := Style{FG: BrightBlack, BG: Default, Italic: true, Link: -1}
		l.emit(Line{Spans: append(clone(first), Span{Text: lang, Style: label, Src: -1}), Src: start})
		first = rest
	}
	avail := l.avail(append(clone(rest), gutter))
	for i, toks := range highlight(src, lang) {
		lineSrc := -1
		if i < lines.Len() {
			lineSrc = lines.At(i).Start
		}
		p := first
		if i > 0 {
			p = rest
		}
		spans := append(clone(p), gutter)
		w := 0
		for _, t := range toks {
			tw := textWidth(t.Text)
			if w+tw > avail {
				t.Text = runewidthTruncate(t.Text, avail-w)
				spans = append(spans, Span{Text: t.Text, Style: t.Style, Src: lineSrc}, Span{Text: "…", Style: Plain.With(BrightBlack), Src: -1})
				break
			}
			spans = append(spans, Span{Text: t.Text, Style: t.Style, Src: lineSrc})
			w += tw
		}
		l.emit(Line{Spans: spans, Src: lineSrc})
	}
}

// frontmatter draws the YAML frontmatter as a card: keys in a column, values
// beside them, lists joined with commas.
func (l *layout) frontmatter(items yaml.MapSlice) {
	border := Plain.With(BrightBlack)
	key := Style{FG: BrightBlack, BG: Default, Link: -1}
	keyW := 0
	for _, it := range items {
		keyW = max(keyW, textWidth(fmt.Sprint(it.Key)))
	}
	keyW = min(keyW, 16)
	// The card is as wide as its content needs, up to the full width.
	natural := 0
	values := make([]string, len(items))
	for i, it := range items {
		values[i] = yamlValue(it.Value)
		natural = max(natural, keyW+2+textWidth(values[i]))
	}
	inner := min(max(natural, 20), l.width-4)
	title := " frontmatter "
	top := "╭─" + title + strings.Repeat("─", max(inner+2-1-textWidth(title), 0)) + "╮"
	l.emit(Line{Spans: []Span{{Text: top, Style: border, Src: 0}}, Src: 0})
	bar := Span{Text: "│ ", Style: border, Src: -1}
	for i, it := range items {
		k := runewidthTruncate(fmt.Sprint(it.Key), keyW)
		first := []Span{bar, {Text: k + spaces(keyW-textWidth(k)+2), Style: key, Src: 0}}
		rest := []Span{bar, {Text: spaces(keyW + 2), Style: Plain, Src: -1}}
		lines := wrap([]Span{{Text: values[i], Style: Plain, Src: 0}}, inner+2, first, rest, 0)
		for _, ln := range lines {
			pad := inner + 2 - spansWidth(ln.Spans)
			ln.Spans = append(ln.Spans, Span{Text: spaces(pad) + " │", Style: border, Src: -1})
			l.emit(ln)
		}
	}
	l.emit(Line{Spans: []Span{{Text: "╰" + strings.Repeat("─", inner+2) + "╯", Style: border, Src: -1}}, Src: -1})
	l.emit(Line{Src: -1})
}

// yamlValue turns a frontmatter value into one line of text.
func yamlValue(v any) string {
	switch v := v.(type) {
	case []any:
		parts := make([]string, len(v))
		for i, e := range v {
			parts[i] = yamlValue(e)
		}
		return strings.Join(parts, ", ")
	case yaml.MapSlice:
		parts := make([]string, len(v))
		for i, e := range v {
			parts[i] = fmt.Sprint(e.Key) + ": " + yamlValue(e.Value)
		}
		return strings.Join(parts, ", ")
	case nil:
		return "—"
	}
	return strings.TrimSpace(strings.ReplaceAll(fmt.Sprint(v), "\n", " "))
}

func (l *layout) footnotes(n *east.FootnoteList, first, rest []Span) {
	rule := Span{Text: strings.Repeat("─", min(16, l.avail(rest))), Style: Plain.With(BrightBlack), Src: -1}
	l.emit(Line{Spans: append(clone(first), rule), Src: -1})
	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		fn, ok := c.(*east.Footnote)
		if !ok {
			continue
		}
		mark := Span{Text: superscript(fn.Index) + " ", Style: Plain.With(Blue), Src: -1}
		r := append(clone(rest), Span{Text: spaces(textWidth(mark.Text)), Style: Plain, Src: -1})
		l.blocks(fn, append(clone(rest), mark), r, true)
	}
}

func superscript(n int) string {
	const digits = "⁰¹²³⁴⁵⁶⁷⁸⁹"
	sup := []rune(digits)
	var b strings.Builder
	for _, d := range fmt.Sprint(n) {
		b.WriteRune(sup[d-'0'])
	}
	return b.String()
}

// inlines collects the styled text of n's inline children. Text before source
// offset skip is left out (used to drop an alert's [!KIND] marker).
func (l *layout) inlines(n ast.Node, st Style, skip int) []Span {
	var out []Span
	l.inline(n, st, skip, &out)
	// Trim a leading space left behind by a skipped first line.
	for len(out) > 0 && strings.TrimSpace(out[0].Text) == "" {
		out = out[1:]
	}
	return out
}

func (l *layout) inline(n ast.Node, st Style, skip int, out *[]Span) {
	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		switch c := c.(type) {
		case *ast.Text:
			seg := c.Segment
			if seg.Stop <= skip {
				continue
			}
			l.text(string(seg.Value(l.src)), st, seg.Start, out)
			if c.HardLineBreak() {
				*out = append(*out, Span{Text: "\n", Style: st, Src: -1})
			} else if c.SoftLineBreak() {
				*out = append(*out, Span{Text: " ", Style: st, Src: -1})
			}
		case *ast.String:
			*out = append(*out, Span{Text: string(c.Value), Style: st, Src: -1})
		case *ast.CodeSpan:
			var b strings.Builder
			src := -1
			for t := c.FirstChild(); t != nil; t = t.NextSibling() {
				if tx, ok := t.(*ast.Text); ok {
					if src < 0 {
						src = tx.Segment.Start
					}
					b.Write(tx.Segment.Value(l.src))
					if tx.SoftLineBreak() {
						b.WriteByte(' ')
					}
				} else if s, ok := t.(*ast.String); ok {
					b.Write(s.Value)
				}
			}
			cs := st
			cs.FG = Cyan
			cs.Bold = false
			// Code keeps its spaces: use no-break spaces so it isn't wrapped mid-span.
			*out = append(*out, Span{Text: strings.ReplaceAll(b.String(), " ", " "), Style: cs, Src: src})
		case *ast.Emphasis:
			es := st
			if c.Level >= 2 {
				es.Bold = true
			} else {
				es.Italic = true
			}
			l.inline(c, es, skip, out)
		case *east.Strikethrough:
			ss := st
			ss.Strike = true
			ss.Dim = true
			l.inline(c, ss, skip, out)
		case *ast.Link:
			ls := st
			ls.Link = l.addLink(string(c.Destination), string(c.Title))
			ls.Underline = true
			ls.FG = Blue
			l.inline(c, ls, skip, out)
		case *ast.AutoLink:
			url := string(c.URL(l.src))
			ls := st
			ls.Link = l.addLink(url, "")
			ls.Underline = true
			ls.FG = Blue
			*out = append(*out, Span{Text: string(c.Label(l.src)), Style: ls, Src: l.srcOf(c)})
		case *ast.Image:
			pic, _ := l.pictureOf(c)
			link := l.addLink(pic.dest, pic.title)
			*out = append(*out, l.chip(pic, st, link))
		case *ast.RawHTML:
			if pic, ok := l.pictureOf(c); ok {
				*out = append(*out, l.inlineImg(pic, st))
				continue
			}
			hs := st
			hs.Dim = true
			for i := 0; i < c.Segments.Len(); i++ {
				seg := c.Segments.At(i)
				*out = append(*out, Span{Text: string(seg.Value(l.src)), Style: hs, Src: seg.Start})
			}
		case *east.TaskCheckBox:
			// Shown as the list marker instead.
		case *east.FootnoteLink:
			fs := st
			fs.FG = Blue
			*out = append(*out, Span{Text: superscript(c.Index), Style: fs, Src: -1})
		case *east.FootnoteBacklink:
			// Not needed in a terminal.
		default:
			l.inline(c, st, skip, out)
		}
	}
}

// text appends plain text, turning :rex: into the Rex mark.
func (l *layout) text(s string, st Style, src int, out *[]Span) {
	for {
		i := strings.Index(s, ":rex:")
		if i < 0 {
			break
		}
		if i > 0 {
			*out = append(*out, Span{Text: s[:i], Style: st, Src: src})
		}
		*out = append(*out, Span{Text: RexMark, Style: st, Src: src + i})
		s = s[i+5:]
		src += i + 5
	}
	if s != "" {
		*out = append(*out, Span{Text: s, Style: st, Src: src})
	}
}

// RexMark stands in for :rex:. Terminals with kitty graphics will get the
// real icon later.
const RexMark = "🦖"

func (l *layout) addLink(dest, title string) int {
	l.doc.Links = append(l.doc.Links, Link{Dest: dest, Title: title})
	return len(l.doc.Links) - 1
}

func truncate(s string, w int) string {
	if textWidth(s) <= w {
		return s
	}
	return runewidthTruncate(s, w-1) + "…"
}
