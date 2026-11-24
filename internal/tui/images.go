package tui

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"golang.org/x/image/draw"
	_ "golang.org/x/image/webp"
	"golang.org/x/sys/unix"

	"github.com/travelingflwr/rex-doc/internal/doc"
	"github.com/travelingflwr/rex-doc/internal/rex"
)

// Images are never sent at full quality: each figure gets a copy scaled to
// the size it's shown at (2× the cell size, for sharp Retina screens),
// cached on disk, made in the background, and sent only when it's near the
// screen.

// thumb is a scaled copy ready to send.
type thumb struct {
	id         uint32
	cols, rows int
	png        []byte
	err        error
}

// imageState tracks which figures were asked for and sent.
type imageState struct {
	requested map[uint32]bool
	sent      map[uint32]bool
	ready     chan thumb
}

func newImageState() *imageState {
	return &imageState{requested: map[uint32]bool{}, sent: map[uint32]bool{}, ready: make(chan thumb, 8)}
}

// requestNearby starts scaling the figures within a screen of the view.
func (v *viewer) requestNearby() {
	if !v.rich || v.images == nil {
		return
	}
	lo, hi := v.top-v.rows(), v.top+2*v.rows()
	for _, f := range v.doc.Figures {
		if f.Line+f.Rows < lo || f.Line > hi || v.images.requested[f.ID] {
			continue
		}
		v.images.requested[f.ID] = true
		f := f
		cw, ch := v.cellW, v.cellH
		go func() {
			data, err := scaledCopy(f.Path, min(f.Cols*cw*2, f.Width), min(f.Rows*ch*2, f.Height))
			v.images.ready <- thumb{id: f.ID, cols: f.Cols, rows: f.Rows, png: data, err: err}
		}()
	}
}

// sendImage transmits a scaled copy as a virtual placement for its
// placeholder cells.
func (v *viewer) sendImage(t thumb) {
	if t.err != nil {
		return
	}
	b64 := base64.StdEncoding.EncodeToString(t.png)
	var b strings.Builder
	for i := 0; i < len(b64); i += 4096 {
		end := min(i+4096, len(b64))
		more := 0
		if end < len(b64) {
			more = 1
		}
		if i == 0 {
			fmt.Fprintf(&b, "\x1b_Ga=T,f=100,U=1,i=%d,c=%d,r=%d,q=2,m=%d;%s\x1b\\", t.id, t.cols, t.rows, more, b64[i:end])
		} else {
			fmt.Fprintf(&b, "\x1b_Gm=%d;%s\x1b\\", more, b64[i:end])
		}
	}
	v.write(b.String())
	v.images.sent[t.id] = true
}

// forgetImages deletes images the current layout no longer uses, after a
// resize or a switch to another file.
func (v *viewer) forgetImages() {
	if v.images == nil {
		return
	}
	keep := map[uint32]bool{}
	for _, f := range v.doc.Figures {
		keep[f.ID] = true
	}
	for id := range v.images.sent {
		if !keep[id] {
			v.write(fmt.Sprintf("\x1b_Ga=d,d=I,i=%d,q=2\x1b\\", id))
			delete(v.images.sent, id)
		}
	}
	for id := range v.images.requested {
		if !keep[id] {
			delete(v.images.requested, id)
		}
	}
}

// scaledCopy returns path scaled to fit w×h pixels as PNG, from the cache
// when it was made before.
func scaledCopy(path string, w, h int) ([]byte, error) {
	st, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	key := sha256.Sum256([]byte(fmt.Sprintf("%s|%d|%d|%d|%d", path, st.ModTime().UnixNano(), st.Size(), w, h)))
	cache := ""
	if dir, err := os.UserCacheDir(); err == nil {
		cache = filepath.Join(dir, "rdxt", "images", hex.EncodeToString(key[:16])+".png")
		if data, err := os.ReadFile(cache); err == nil {
			return data, nil
		}
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	src, _, err := image.Decode(f)
	if err != nil {
		return nil, err
	}
	b := src.Bounds()
	// Fit inside w×h, keeping proportions, never larger than the original.
	scale := min(float64(w)/float64(b.Dx()), float64(h)/float64(b.Dy()), 1)
	dw, dh := max(int(float64(b.Dx())*scale), 1), max(int(float64(b.Dy())*scale), 1)
	dst := image.NewNRGBA(image.Rect(0, 0, dw, dh))
	draw.CatmullRom.Scale(dst, dst.Bounds(), src, b, draw.Src, nil)
	var out bytes.Buffer
	enc := png.Encoder{CompressionLevel: png.BestSpeed}
	if err := enc.Encode(&out, dst); err != nil {
		return nil, err
	}
	if cache != "" {
		if os.MkdirAll(filepath.Dir(cache), 0o755) == nil {
			os.WriteFile(cache, out.Bytes(), 0o644)
		}
	}
	return out.Bytes(), nil
}

// cellSize returns the terminal's cell size in pixels from the window size
// the terminal reports (Rex fills in its pixel size), or 8×16.
func cellSize(fd int) (int, int) {
	ws, err := unix.IoctlGetWinsize(fd, unix.TIOCGWINSZ)
	if err != nil || ws.Col == 0 || ws.Row == 0 || ws.Xpixel == 0 || ws.Ypixel == 0 {
		return 8, 16
	}
	return int(ws.Xpixel) / int(ws.Col), int(ws.Ypixel) / int(ws.Row)
}

// showImage opens a closer look at an image: a floating Rex window with
// rdxt's image view, or the system's viewer outside Rex.
func (v *viewer) showImage(path string) {
	if p, err := pathUnescape(path); err == nil {
		path = p
	}
	if !filepath.IsAbs(path) {
		path = filepath.Join(filepath.Dir(v.path), path)
	}
	if _, err := os.Stat(path); err != nil {
		v.setFlash("image not found")
		return
	}
	if v.rich {
		self, _ := os.Executable()
		args, _ := json.Marshal(map[string]any{
			"bounds": []float64{0.08, 0.06, 0.84, 0.88},
			"focus":  true,
			"layout": map[string]any{"block": map[string]any{
				"flavor": "com.superlogical.terminal.shell", "label": filepath.Base(path),
				"options": map[string]any{"command": []string{self, "--image", path}},
			}},
		})
		if exec.Command("rex", "api", "call", "-s", rex.Here(), "session.new_layer", string(args)).Run() == nil {
			return
		}
	}
	if openExternal(path) != nil {
		v.setFlash("can't open the image")
	}
}

var _ = doc.Placeholder
