package doc

import (
	"strings"
	"testing"
)

func textLines(d *Doc) []string {
	var out []string
	for _, ln := range d.Lines {
		var b strings.Builder
		for _, s := range ln.Spans {
			b.WriteString(s.Text)
		}
		out = append(out, b.String())
	}
	return out
}

func TestTextKeepsLinesAndSpacing(t *testing.T) {
	src := "# not a heading\n\nName    Size\nfoo.go   12\n*not* `code`\n"
	got := textLines(Render([]byte(src), 100, Options{Text: true}))
	want := []string{"# not a heading", "", "Name    Size", "foo.go   12", "*not* `code`"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestTextTabsGoToTabStops(t *testing.T) {
	got := textLines(Render([]byte("a\tb\n\tc\n"), 100, Options{Text: true}))
	if got[0] != "a       b" || got[1] != "        c" {
		t.Errorf("got %q", got)
	}
}

func TestTextLongLinesWrapWithTheirIndent(t *testing.T) {
	long := "    " + strings.Repeat("word ", 30)
	d := Render([]byte(long+"\n"), 60, Options{Text: true})
	lines := textLines(d)
	if len(lines) < 3 {
		t.Fatalf("got %d lines, want the long line wrapped", len(lines))
	}
	for _, ln := range lines {
		if textWidth(ln) > d.Width || !strings.HasPrefix(ln, "    word") {
			t.Errorf("line %q is too wide or lost its indent", ln)
		}
	}
}

func TestTextSourceOffsets(t *testing.T) {
	src := "one\ntwo\n\nthree\n"
	d := Render([]byte(src), 100, Options{Text: true})
	for _, ln := range d.Lines {
		for _, s := range ln.Spans {
			if s.Src >= 0 && src[s.Src:s.Src+len(s.Text)] != s.Text {
				t.Errorf("span %q points at %q", s.Text, src[s.Src:s.Src+len(s.Text)])
			}
		}
	}
}
