package doc

import (
	"html"
	"image"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/yuin/goldmark/ast"
)

// picture is an image the document asks for, written either as Markdown
// (![alt](path)) or as an HTML <img> tag.
type picture struct {
	dest, alt, title string
	// width and height are the size the tag asks for in CSS pixels, or 0.
	width, height int
	src           int
}

var (
	imgTag   = regexp.MustCompile(`(?is)<img\b[^>]*>`)
	anyTag   = regexp.MustCompile(`(?s)<[^>]*>`)
	htmlAttr = regexp.MustCompile(`(?s)([a-zA-Z][a-zA-Z0-9-]*)\s*=\s*(?:"([^"]*)"|'([^']*)'|([^\s"'>]+))`)
)

// cssCell is how many CSS pixels one cell stands for when a tag gives a
// width: about what a browser shows at the terminal's reading size.
const cssCell = 8

// parseImg reads one <img> tag.
func parseImg(tag string, src int) (picture, bool) {
	tag = strings.TrimSpace(tag)
	if loc := imgTag.FindStringIndex(tag); loc == nil || loc[0] != 0 || loc[1] != len(tag) {
		return picture{}, false
	}
	p := picture{src: src}
	for _, m := range htmlAttr.FindAllStringSubmatch(tag[4:], -1) {
		val := html.UnescapeString(m[2] + m[3] + m[4])
		switch strings.ToLower(m[1]) {
		case "src":
			p.dest = strings.TrimSpace(val)
		case "alt":
			p.alt = strings.TrimSpace(val)
		case "title":
			p.title = strings.TrimSpace(val)
		case "width":
			p.width = cssPixels(val)
		case "height":
			p.height = cssPixels(val)
		}
	}
	return p, p.dest != ""
}

// cssPixels reads "200" or "200px". Percentages and other units are 0.
func cssPixels(s string) int {
	n, err := strconv.Atoi(strings.TrimSuffix(strings.TrimSpace(strings.ToLower(s)), "px"))
	if err != nil || n < 0 {
		return 0
	}
	return n
}

func rawHTML(n *ast.RawHTML, src []byte) string {
	var b strings.Builder
	for i := 0; i < n.Segments.Len(); i++ {
		seg := n.Segments.At(i)
		b.Write(seg.Value(src))
	}
	return b.String()
}

// pictureOf returns the image an inline node stands for, if it is one.
func (l *layout) pictureOf(n ast.Node) (picture, bool) {
	switch n := n.(type) {
	case *ast.Image:
		return picture{dest: strings.TrimSpace(string(n.Destination)), alt: plainText(l.inlines(n, Plain, -1)),
			title: strings.TrimSpace(string(n.Title)), src: l.srcOf(n)}, true
	case *ast.RawHTML:
		src := -1
		if n.Segments.Len() > 0 {
			src = n.Segments.At(0).Start
		}
		return parseImg(rawHTML(n, l.src), src)
	}
	return picture{}, false
}

// soloPicture returns the image when a paragraph (or table cell) holds
// nothing but one image.
func (l *layout) soloPicture(p ast.Node) (picture, bool) {
	var pic picture
	found := false
	for c := p.FirstChild(); c != nil; c = c.NextSibling() {
		if t, ok := c.(*ast.Text); ok {
			if strings.TrimSpace(string(t.Segment.Value(l.src))) != "" {
				return picture{}, false
			}
			continue
		}
		q, ok := l.pictureOf(c)
		if !ok || found {
			return picture{}, false
		}
		pic, found = q, true
	}
	return pic, found
}

// blockPictures returns the images of an HTML block that holds only images
// (perhaps wrapped in <p align="center"> or <a>), and nil for any other.
func blockPictures(text string, src int) []picture {
	if strings.TrimSpace(anyTag.ReplaceAllString(text, "")) != "" {
		return nil
	}
	var pics []picture
	for _, loc := range imgTag.FindAllStringIndex(text, -1) {
		if p, ok := parseImg(text[loc[0]:loc[1]], src+loc[0]); ok {
			pics = append(pics, p)
		}
	}
	return pics
}

func isRemote(dest string) (host string, ok bool) {
	u, err := url.Parse(dest)
	if err == nil && (u.Scheme == "http" || u.Scheme == "https") {
		return u.Host, true
	}
	return "", false
}

// localPath is where a relative or absolute image path is on disk.
func (l *layout) localPath(dest string) string {
	path := dest
	if p, err := url.PathUnescape(path); err == nil {
		path = p
	}
	if !filepath.IsAbs(path) && l.opts.BaseDir != "" {
		path = filepath.Join(l.opts.BaseDir, path)
	}
	return path
}

// readable returns a local image's path and size when it can be drawn.
func (l *layout) readable(dest string) (string, image.Config, bool) {
	if _, remote := isRemote(dest); remote {
		return "", image.Config{}, false
	}
	path := l.localPath(dest)
	f, err := os.Open(path)
	if err != nil {
		return "", image.Config{}, false
	}
	defer f.Close()
	cfg, _, err := image.DecodeConfig(f)
	if err != nil || cfg.Width == 0 || cfg.Height == 0 {
		return "", image.Config{}, false
	}
	return path, cfg, true
}

// fitCells sizes an image in cells: at most maxCols wide and maxRows tall,
// never scaled up, keeping its shape.
func (l *layout) fitCells(cfg image.Config, maxCols, maxRows int) (cols, rows int) {
	cw, ch := max(l.opts.CellW, 1), max(l.opts.CellH, 1)
	cols = max(min(maxCols, (cfg.Width+cw-1)/cw), 1)
	rows = max(int(float64(cols*cw)*float64(cfg.Height)/float64(cfg.Width)/float64(ch)+0.5), 1)
	if rows > maxRows {
		rows = max(maxRows, 1)
		cols = max(int(float64(rows*ch)*float64(cfg.Width)/float64(cfg.Height)/float64(cw)+0.5), 1)
	}
	return cols, rows
}

func (l *layout) maxFigureRows() int {
	return min(max(l.opts.ScreenRows/3, 4), MaxFigureRows)
}

// chip is an image shown as text: 🖼 and its alt text, as a link to it.
func (l *layout) chip(p picture, st Style, link int) Span {
	st.Link = link
	st.FG = Blue
	st.Italic = true
	alt := p.alt
	if alt == "" {
		alt = "image"
	}
	return Span{Text: "🖼 " + alt, Style: st, Src: p.src}
}

// inlineImg is an <img> inside a sentence or heading: a chip, like a
// Markdown image there.
func (l *layout) inlineImg(p picture, st Style) Span {
	link := l.addLink(p.dest, p.title)
	l.doc.Links[link].Image = true
	return l.chip(p, st, link)
}

// cellImage is an image that fills a table cell.
type cellImage struct {
	path    string
	cfg     image.Config
	link    int
	src     int
	natural int // the widest it wants to be, in cells
}

// cellPicture returns the image to draw in a table cell that holds nothing
// but one image.
func (l *layout) cellPicture(c ast.Node) (picture, *cellImage, bool) {
	p, ok := l.soloPicture(c)
	if !ok {
		return picture{}, nil, false
	}
	path, cfg, readable := l.readable(p.dest)
	if !readable || !l.opts.Images {
		return p, nil, true
	}
	want := 40
	if p.width > 0 {
		want = max(p.width/cssCell, 4)
	}
	cols, _ := l.fitCells(cfg, want, l.maxFigureRows())
	return p, &cellImage{path: path, cfg: cfg, src: p.src, natural: cols}, true
}

// lines lays a cell's image out width cells wide, starting at line.
func (l *layout) cellImageLines(ci *cellImage, width, line int) []Line {
	cols, rows := l.fitCells(ci.cfg, min(width, ci.natural), l.maxFigureRows())
	id := figureID(ci.path, cols, rows)
	st := Style{FG: Default, BG: Default, Link: ci.link, Image: id}
	out := make([]Line, rows)
	for r := range out {
		out[r] = Line{Spans: []Span{{Text: PlaceholderRow(r, cols), Style: st, Src: -1}}, Src: -1}
	}
	out[0].Src = ci.src
	l.doc.Figures = append(l.doc.Figures, Figure{ID: id, Path: ci.path, Cols: cols, Rows: rows, Line: line,
		Width: ci.cfg.Width, Height: ci.cfg.Height})
	return out
}
