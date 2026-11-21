// Package doc turns Markdown into styled terminal lines.
//
// Every styled run of text keeps the byte offset of the source it came from,
// so callers can keep the reader's place across resizes, reloads and modes.
package doc

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
)

// Color is a terminal color. Default means the terminal's own foreground or
// background. 0–15 are the terminal's 16-color palette, so the output follows
// whatever theme the terminal uses.
type Color int8

const Default Color = -1

const (
	Black Color = iota
	Red
	Green
	Yellow
	Blue
	Magenta
	Cyan
	White
	BrightBlack
	BrightRed
	BrightGreen
	BrightYellow
	BrightBlue
	BrightMagenta
	BrightCyan
	BrightWhite
)

// Style is how a run of text looks.
type Style struct {
	FG        Color
	BG        Color
	Bold      bool
	Dim       bool
	Italic    bool
	Underline bool
	Strike    bool
	// Link is the index of the link this text belongs to in Doc.Links, or -1.
	Link int
	// Image, when not 0, is a kitty image ID: the text is placeholder cells
	// and the ID is sent as its 24-bit foreground color.
	Image uint32
}

// Plain is the default style.
var Plain = Style{FG: Default, BG: Default, Link: -1}

// With returns a copy of s with fg set.
func (s Style) With(fg Color) Style { s.FG = fg; return s }

// SGR returns the escape sequence that switches to s from the reset state.
func (s Style) SGR() string {
	var codes []string
	if s.Bold {
		codes = append(codes, "1")
	}
	if s.Dim {
		codes = append(codes, "2")
	}
	if s.Italic {
		codes = append(codes, "3")
	}
	if s.Underline {
		codes = append(codes, "4")
	}
	if s.Strike {
		codes = append(codes, "9")
	}
	if s.Image != 0 {
		codes = append(codes, fmt.Sprintf("38;2;%d;%d;%d", s.Image>>16&0xFF, s.Image>>8&0xFF, s.Image&0xFF))
	} else if s.FG >= 0 {
		if s.FG < 8 {
			codes = append(codes, strconv.Itoa(30+int(s.FG)))
		} else {
			codes = append(codes, strconv.Itoa(90+int(s.FG)-8))
		}
	}
	if s.BG >= 0 {
		if s.BG < 8 {
			codes = append(codes, strconv.Itoa(40+int(s.BG)))
		} else {
			codes = append(codes, strconv.Itoa(100+int(s.BG)-8))
		}
	}
	if len(codes) == 0 {
		return ""
	}
	return "\x1b[" + strings.Join(codes, ";") + "m"
}

// Span is a run of text in one style.
type Span struct {
	Text  string
	Style Style
	// Src is the byte offset in the source where this text starts, or -1 for
	// decoration such as bullets and rules.
	Src int
}

// Line is one row of output.
type Line struct {
	Spans []Span
	// Src is the source offset of the first real text on the line (or of the
	// block it belongs to). Used to keep the reader's place.
	Src int
	// Heading is the heading level (1–6) if this line is a heading's text.
	Heading int
	// TaskCols is set on the first line of a task-list item: the column
	// (after the margin) where its marker ends, for clicks. 0 otherwise.
	TaskCols int
	// Task is the source offset of the task item's text, when TaskCols > 0.
	// Its checkbox is just before it on the same source line.
	Task int
}

// Link is a link target found in the document.
type Link struct {
	Dest  string
	Title string
	// Image is true for an image's link: following it shows the image.
	Image bool
}

// Heading is an entry in the document outline.
type Heading struct {
	Level int
	Text  string
	Slug  string
	Line  int // index into Doc.Lines
}

// Doc is a laid-out document.
type Doc struct {
	Lines    []Line
	Links    []Link
	Headings []Heading
	// Margin is the number of blank columns to the left of every line.
	Margin int
	// Width is the content width the document was laid out for.
	Width int
	// HasFrontmatter is true when the document starts with YAML frontmatter,
	// whether or not it is shown.
	HasFrontmatter bool
	// Figures are the images drawn as figures.
	Figures []Figure
}

// Options change how a document is laid out.
type Options struct {
	// ShowFrontmatter shows the frontmatter card at the top.
	ShowFrontmatter bool
	// Dok lays the source out as a Dok document instead of Markdown.
	Dok bool
	// Text lays the source out as plain text: its lines as they are,
	// wrapped when too long, with nothing read as markup.
	Text bool
	// Images draws figures with kitty graphics placeholders (in Rex).
	// Without it, figures are cards.
	Images bool
	// CellW and CellH are the terminal's cell size in pixels, and
	// ScreenRows its height, for sizing figures.
	CellW, CellH, ScreenRows int
	// BaseDir is where relative image paths start from.
	BaseDir string
}

// OptionsFor returns the options a file needs, by its extension.
func OptionsFor(path string) Options {
	ext := strings.ToLower(filepath.Ext(path))
	return Options{Dok: ext == ".dok", Text: ext == ".txt", BaseDir: filepath.Dir(path),
		CellW: 8, CellH: 16, ScreenRows: 40}
}

// LineForSrc returns the index of the first line at or after source offset src.
func (d *Doc) LineForSrc(src int) int {
	best := 0
	for i, l := range d.Lines {
		if l.Src < 0 {
			continue
		}
		if l.Src >= src {
			return i
		}
		best = i
	}
	return best
}

// SrcForLine returns the source offset for line i, searching backwards past
// decoration lines.
func (d *Doc) SrcForLine(i int) int {
	for ; i >= 0 && i < len(d.Lines); i-- {
		if d.Lines[i].Src >= 0 {
			return d.Lines[i].Src
		}
	}
	return 0
}
