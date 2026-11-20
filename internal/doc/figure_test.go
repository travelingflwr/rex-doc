package doc

import (
	"strings"
	"testing"
)

func figureOpts() Options {
	return Options{Images: true, CellW: 8, CellH: 16, ScreenRows: 48, BaseDir: "../../testdata"}
}

func TestFiguresKeepTheirShape(t *testing.T) {
	d := Render([]byte("![wide](images/landscape.png)\n"), 100, figureOpts())
	if len(d.Figures) != 1 {
		t.Fatalf("got %d figures, want 1", len(d.Figures))
	}
	f := d.Figures[0]
	// 1600×900 at 8×16 px cells: rows ≈ cols × 8 × 900 / 1600 / 16.
	want := float64(f.Cols) * 8 * 900 / 1600 / 16
	if diff := float64(f.Rows) - want; diff > 1 || diff < -1 {
		t.Errorf("figure %d×%d cells, want about %.1f rows for its shape", f.Cols, f.Rows, want)
	}
}

func TestTallFiguresAreCapped(t *testing.T) {
	d := Render([]byte("![tall](images/tree.png)\n"), 100, figureOpts())
	f := d.Figures[0]
	if f.Rows > 48/3 {
		t.Errorf("tall figure is %d rows, want at most a third of 48", f.Rows)
	}
	want := float64(f.Rows) * 16 * 600 / 1200 / 8
	if diff := float64(f.Cols) - want; diff > 1 || diff < -1 {
		t.Errorf("capped figure %d×%d cells lost its shape (want about %.1f cols)", f.Cols, f.Rows, want)
	}
}

func TestSmallImagesAreNotScaledUp(t *testing.T) {
	d := Render([]byte("![icon](images/icon.png)\n"), 100, figureOpts())
	if f := d.Figures[0]; f.Cols > 6 {
		t.Errorf("48 px icon is %d cells wide, want at most 6 (48/8)", f.Cols)
	}
}

func TestInlineImagesAreChips(t *testing.T) {
	d := Render([]byte("Text ![badge](images/icon.png) more.\n"), 100, figureOpts())
	if len(d.Figures) != 0 {
		t.Errorf("an image inside a sentence became a figure")
	}
}

func TestMissingAndRemoteImagesAreCards(t *testing.T) {
	d := Render([]byte("![a](missing.png)\n\n![b](https://example.com/x.png)\n"), 100, figureOpts())
	if len(d.Figures) != 0 {
		t.Errorf("missing or remote images became figures")
	}
}

func TestImgTagOnItsOwnLineIsAFigure(t *testing.T) {
	for _, src := range []string{
		"<img src=\"images/landscape.png\" alt=\"wide\">\n",
		"<p align=\"center\">\n  <img src='images/landscape.png' alt=\"wide\" width=\"400\">\n</p>\n",
		"Text.\n\n<img src=images/landscape.png>\n",
	} {
		d := Render([]byte(src), 100, figureOpts())
		if len(d.Figures) != 1 {
			t.Errorf("%q: got %d figures, want 1", src, len(d.Figures))
		}
	}
}

func TestImgTagWidthLimitsTheFigure(t *testing.T) {
	d := Render([]byte("<img src=\"images/landscape.png\" width=\"160\">\n"), 100, figureOpts())
	if f := d.Figures[0]; f.Cols > 20 {
		t.Errorf("width=160 figure is %d cells wide, want at most 20", f.Cols)
	}
}

func TestImgTagsInTableCellsAreDrawn(t *testing.T) {
	src := "| A | B |\n|---|---|\n| <img src=\"images/tree.png\" alt=\"t\" width=\"200\"> | <img src=\"images/landscape.png\" width=\"200\"> |\n"
	d := Render([]byte(src), 80, figureOpts())
	if len(d.Figures) != 2 {
		t.Fatalf("got %d figures, want 2", len(d.Figures))
	}
	// Every line of the table is the same width: image cells fill their columns.
	w := -1
	for _, ln := range d.Lines {
		if lw := spansWidth(ln.Spans); w < 0 {
			w = lw
		} else if lw != w {
			t.Fatalf("table lines are %d and %d cells wide", w, lw)
		}
	}
	for _, f := range d.Figures {
		if f.Line < 3 || f.Line >= len(d.Lines) {
			t.Errorf("figure starts at line %d, outside the table body", f.Line)
		}
	}
}

func TestImgTagsWithoutGraphicsAreText(t *testing.T) {
	opts := figureOpts()
	opts.Images = false
	src := "# <img src=\"images/icon.png\" alt=\"icon\" height=\"40\"> Title\n\n| A |\n|---|\n| <img src=\"images/tree.png\" alt=\"a tree\"> |\n\nBig <img src=\"images/tree.png\" alt=\"tree\"> inline.\n"
	d := Render([]byte(src), 100, opts)
	if len(d.Figures) != 0 {
		t.Errorf("got %d figures without graphics", len(d.Figures))
	}
	var all string
	for _, ln := range d.Lines {
		for _, s := range ln.Spans {
			all += s.Text
		}
	}
	if strings.Contains(all, "<img") || !strings.Contains(all, "🖼 a tree") {
		t.Errorf("img tags weren't turned into chips:\n%s", all)
	}
}

func TestOtherHTMLIsLeftAlone(t *testing.T) {
	d := Render([]byte("<details>\n<summary>More</summary>\n<img src=\"images/icon.png\">\n</details>\n"), 100, figureOpts())
	if len(d.Figures) != 0 {
		t.Errorf("an HTML block with text became figures")
	}
}
