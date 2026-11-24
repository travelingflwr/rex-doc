package tui

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/travelingflwr/rex-doc/internal/rex"
)

// inRex reports whether rdxt is running in a Rex terminal.
func inRex() bool {
	return os.Getenv("TERM_PROGRAM") == "rex" || rex.Here() != ""
}

// rexFloating asks Rex whether rdxt's own block is in a floating layer. It
// runs a rex command, so call it off the main loop.
func rexFloating() bool {
	session, block := rex.Here(), os.Getenv("REX_BLOCK")
	if session == "" || block == "" {
		return false
	}
	out, err := exec.Command("rex", "api", "call", "-s", session, "session.view").Output()
	if err != nil {
		return false
	}
	var view struct {
		Windows []struct {
			Layers []struct {
				Kind   string          `json:"kind"`
				Layout json.RawMessage `json:"layout"`
			} `json:"layers"`
		} `json:"windows"`
	}
	if json.Unmarshal(out, &view) != nil {
		return false
	}
	for _, w := range view.Windows {
		for _, l := range w.Layers {
			if l.Kind == "floating" && strings.Contains(string(l.Layout), `"`+block+`"`) {
				return true
			}
		}
	}
	return false
}

// rexLabel names rdxt's block after the document, so Rex's pane header
// shows it. It's best effort and runs in the background.
func rexLabel(path string) {
	block := os.Getenv("REX_BLOCK")
	if block == "" {
		return
	}
	cmd := exec.Command("rex", "block", "rename", block, filepath.Base(path))
	cmd.Stdout, cmd.Stderr = nil, nil
	go cmd.Run()
}

// rexView returns Rex's session view, decoded loosely.
func rexView() map[string]any {
	session := rex.Here()
	if session == "" {
		return nil
	}
	out, err := exec.Command("rex", "api", "call", "-s", session, "session.view").Output()
	if err != nil {
		return nil
	}
	var v map[string]any
	if json.Unmarshal(out, &v) != nil {
		return nil
	}
	return v
}

// blockIDs collects every block ID in a layout tree.
func blockIDs(n any, out *[]string) {
	switch n := n.(type) {
	case map[string]any:
		if id, ok := n["block_id"].(string); ok {
			*out = append(*out, id)
		}
		for _, c := range n {
			blockIDs(c, out)
		}
	case []any:
		for _, c := range n {
			blockIDs(c, out)
		}
	}
}

// rexDock moves rdxt's floating window into the tab's layout, beside the
// last pane there. It reports whether it worked.
func rexDock() bool {
	block := os.Getenv("REX_BLOCK")
	v := rexView()
	if v == nil || block == "" {
		return false
	}
	windows, _ := v["windows"].([]any)
	for _, w := range windows {
		wm, _ := w.(map[string]any)
		layers, _ := wm["layers"].([]any)
		var mine bool
		var anchor string
		for _, l := range layers {
			lm, _ := l.(map[string]any)
			var ids []string
			blockIDs(lm["layout"], &ids)
			if lm["kind"] == "floating" && containsStr(ids, block) {
				mine = true
			}
			if lm["kind"] == "tiled" && len(ids) > 0 {
				anchor = ids[len(ids)-1]
			}
		}
		if mine && anchor != "" {
			body, _ := json.Marshal(map[string]any{"block_id": block, "anchor_block_id": anchor, "direction": "horizontal"})
			return exec.Command("rex", "api", "call", "-s", rex.Here(), "session.move_block", string(body)).Run() == nil
		}
	}
	return false
}

// rexFloat moves rdxt's pane out of the layout into a floating window.
func rexFloat() bool {
	block := os.Getenv("REX_BLOCK")
	if block == "" {
		return false
	}
	body, _ := json.Marshal(map[string]any{"block_id": block, "bounds": []float64{0.1, 0.06, 0.8, 0.88}})
	return exec.Command("rex", "api", "call", "-s", rex.Here(), "session.float_block", string(body)).Run() == nil
}

func containsStr(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}
