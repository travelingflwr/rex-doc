package doc

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"image"
	_ "image/png"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/travelingflwr/rex-doc/internal/mermaid"
)

// DiagramsReady counts Mermaid images finished in the background. The
// viewer lays the document out again when it changes.
var DiagramsReady atomic.Int64

var diagramRoles = map[mermaid.Role]Style{
	mermaid.Plain:      Plain,
	mermaid.Border:     Plain.With(Blue),
	mermaid.Name:       {FG: Default, BG: Default, Bold: true, Link: -1},
	mermaid.Edge:       Plain,
	mermaid.Arrow:      {FG: Default, BG: Default, Bold: true, Link: -1},
	mermaid.Label:      {FG: Default, BG: Default, Italic: true, Link: -1},
	mermaid.Dashed:     Plain.With(BrightBlack),
	mermaid.Note:       Plain.With(Yellow),
	mermaid.Frame:      Plain.With(Magenta),
	mermaid.FrameLabel: {FG: Magenta, BG: Default, Bold: true, Link: -1},
	mermaid.Decision:   Plain.With(Yellow),
	mermaid.Dim:        Plain.With(BrightBlack),
	mermaid.Bar1:       Plain.With(Blue),
	mermaid.Bar2:       Plain.With(Green),
	mermaid.Bar3:       Plain.With(Magenta),
	mermaid.Bar4:       Plain.With(Yellow),
	mermaid.Bar5:       Plain.With(Cyan),
	mermaid.Bar6:       Plain.With(Red),
}

// diagram draws a ```mermaid block. It reports false when the block should
// be shown as code instead, after adding a note that says why.
func (l *layout) diagram(code string, src int, first, rest []Span) bool {
	lines, err := mermaid.Render(code, l.avail(rest))
	if err == nil {
		p := first
		for _, ml := range lines {
			spans := clone(p)
			for _, r := range ml {
				spans = append(spans, Span{Text: r.Text, Style: diagramRoles[r.Role], Src: -1})
			}
			l.emit(Line{Spans: spans, Src: src})
			p = rest
		}
		return true
	}
	kind := mermaid.Kind(code)
	// A picture from mermaid-cli, when it's installed and we're in Rex.
	if l.opts.Images && (errors.Is(err, mermaid.ErrUnsupported) || errors.Is(err, mermaid.ErrTooWide)) {
		if png, ok := mermaidImage(code); ok {
			if f, err := os.Open(png); err == nil {
				cfg, _, err := image.DecodeConfig(f)
				f.Close()
				if err == nil {
					l.figures++
					link := l.addLink(png, "")
					l.doc.Links[link].Image = true
					l.placeFigure(png, cfg, 0, mermaid.KindName(kind), link, src, first, rest)
					return true
				}
			}
		}
	}
	why := "shown as code"
	switch {
	case errors.Is(err, mermaid.ErrTooWide):
		why = "too wide for this pane, so shown as code; widen the pane to see it drawn"
	case errors.Is(err, mermaid.ErrUnsupported) && mermaidPending(code):
		why = "drawing it…"
	case errors.Is(err, mermaid.ErrUnsupported):
		why = "shown as code (install mermaid-cli, mmdc, to see it as a picture)"
	}
	note := Style{FG: BrightBlack, BG: Default, Italic: true, Link: -1}
	l.emit(Line{Spans: append(clone(first), Span{Text: mermaid.KindName(kind) + " · " + why, Style: note, Src: src}), Src: src})
	return false
}

var (
	mmdcOnce sync.Once
	mmdcPath string
	pending  sync.Map // cache path → true while being drawn
)

func mermaidCache(code string) string {
	dir, err := os.UserCacheDir()
	if err != nil {
		return ""
	}
	sum := sha256.Sum256([]byte(code))
	return filepath.Join(dir, "rdxt", "mermaid", hex.EncodeToString(sum[:12])+".png")
}

func mermaidPending(code string) bool {
	_, ok := pending.Load(mermaidCache(code))
	return ok
}

// mermaidImage returns a cached PNG of the diagram. When there's none yet
// and mmdc is installed, it starts drawing one in the background and bumps
// DiagramsReady when it's done.
func mermaidImage(code string) (string, bool) {
	out := mermaidCache(code)
	if out == "" {
		return "", false
	}
	if _, err := os.Stat(out); err == nil {
		return out, true
	}
	mmdcOnce.Do(func() { mmdcPath, _ = exec.LookPath("mmdc") })
	if mmdcPath == "" {
		return "", false
	}
	if _, busy := pending.LoadOrStore(out, true); busy {
		return "", false
	}
	go func() {
		defer pending.Delete(out)
		if os.MkdirAll(filepath.Dir(out), 0o755) != nil {
			return
		}
		in := strings.TrimSuffix(out, ".png") + ".mmd"
		if os.WriteFile(in, []byte(code), 0o644) != nil {
			return
		}
		cmd := exec.Command(mmdcPath, "-i", in, "-o", out, "-t", "dark", "-b", "transparent", "-s", "2")
		if cmd.Run() == nil {
			DiagramsReady.Add(1)
		}
		os.Remove(in)
	}()
	return "", false
}
