package rex

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/travelingflwr/rex-doc/internal/doc"
)

// ConfigPath is Rex's config file: $REX_CONFIG, or init.lua in Rex's
// folder under $XDG_CONFIG_HOME or ~/.config.
func ConfigPath() string {
	if c := os.Getenv("REX_CONFIG"); c != "" {
		return c
	}
	dir := os.Getenv("XDG_CONFIG_HOME")
	if dir == "" {
		home, _ := os.UserHomeDir()
		dir = filepath.Join(home, ".config")
	}
	return filepath.Join(dir, "rex", "init.lua")
}

// IsConfig reports whether path is Rex's own config file. Only that file is
// checked: checking runs its Lua, and rdxt never runs a document's code.
func IsConfig(path string) bool {
	return samePath(path, ConfigPath())
}

func samePath(a, b string) bool {
	ra, err1 := filepath.EvalSymlinks(a)
	rb, err2 := filepath.EvalSymlinks(b)
	if err1 != nil || err2 != nil {
		return filepath.Clean(a) == filepath.Clean(b)
	}
	return ra == rb
}

// CheckConfig asks Rex to load its config file (rex config check) and
// returns what it found.
func CheckConfig() *doc.Check {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	// Exit status 1 means the config has errors; the report is still there.
	out, err := exec.CommandContext(ctx, Bin(), "config", "check", "--json").Output()
	var r struct {
		Bindings    int               `json:"bindings"`
		Removals    int               `json:"removals"`
		Modes       int               `json:"modes"`
		Actions     []json.RawMessage `json:"actions"`
		Diagnostics []struct {
			Severity string `json:"severity"`
			Source   string `json:"source"`
			Message  string `json:"message"`
		} `json:"diagnostics"`
	}
	if jerr := json.Unmarshal(out, &r); jerr != nil {
		msg := "rex config check didn't answer"
		if err != nil {
			msg += ": " + err.Error()
		}
		return &doc.Check{Err: msg}
	}
	c := &doc.Check{Summary: summary(r.Bindings, r.Removals, r.Modes, len(r.Actions))}
	cfg := ConfigPath()
	for _, d := range r.Diagnostics {
		c.Problems = append(c.Problems, doc.Problem{Severity: d.Severity, Where: d.Source,
			Line: lineIn(d.Source, cfg), Message: d.Message})
	}
	return c
}

// lineIn returns the line a diagnostic's source ("init.lua:3",
// "plugins/x/init.lua:20") points at when it's in file, or 0.
func lineIn(source, file string) int {
	i := strings.LastIndexByte(source, ':')
	if i < 0 {
		return 0
	}
	n, err := strconv.Atoi(source[i+1:])
	if err != nil {
		return 0
	}
	p := source[:i]
	if !filepath.IsAbs(p) {
		p = filepath.Join(filepath.Dir(file), p)
	}
	if !samePath(p, file) {
		return 0
	}
	return n
}

func summary(bindings, removals, modes, actions int) string {
	var parts []string
	add := func(n int, one, many string) {
		switch {
		case n == 1:
			parts = append(parts, "1 "+one)
		case n > 1:
			parts = append(parts, fmt.Sprintf("%d %s", n, many))
		}
	}
	add(bindings, "key binding", "key bindings")
	add(removals, "removed binding", "removed bindings")
	add(modes, "key mode", "key modes")
	add(actions, "action", "actions")
	if len(parts) == 0 {
		return "It sets nothing up yet."
	}
	return "It sets up " + strings.Join(parts, ", ") + "."
}
