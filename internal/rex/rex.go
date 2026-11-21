// Package rex opens rdxt in Rex: beside a pane, in a floating window or in
// a new tab. It works from inside a Rex terminal and from outside one (the
// Cmd-click handler), where it finds the session you were last using.
package rex

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

// appRex is where the Rex app keeps its command-line tool.
const appRex = "Library/Application Support/com.superlogical.rex/RexServer/Versions/Current/Rex Server.app/Contents/MacOS/rex"

// Bin returns the rex command: $RDXT_REX, then rex on PATH, then the one
// inside the Rex app.
func Bin() string {
	if b := os.Getenv("RDXT_REX"); b != "" {
		return b
	}
	if b, err := exec.LookPath("rex"); err == nil {
		return b
	}
	if home, err := os.UserHomeDir(); err == nil {
		if p := filepath.Join(home, appRex); fileExists(p) {
			return p
		}
	}
	return "rex"
}

func fileExists(p string) bool { _, err := os.Stat(p); return err == nil }

// Session returns the Rex session to open in: $REX_SESSION inside a Rex
// terminal, or else the one the Rex app last had focused.
func Session() string {
	if s := os.Getenv("REX_SESSION"); s != "" {
		return s
	}
	// The key has a dot in it, so read it from the exported XML.
	out, err := exec.Command("defaults", "export", "com.superlogical.rex", "-").Output()
	if err != nil {
		return ""
	}
	m := focusedRE.FindSubmatch(out)
	if m == nil {
		return ""
	}
	data, err := base64.StdEncoding.DecodeString(strings.Join(strings.Fields(string(m[1])), ""))
	if err != nil {
		return ""
	}
	var v struct {
		SessionID string `json:"sessionID"`
	}
	if json.Unmarshal(data, &v) != nil {
		return ""
	}
	return v.SessionID
}

var focusedRE = regexp.MustCompile(`<key>Workspace\.lastFocusedSession</key>\s*<data>([^<]*)</data>`)

// Available reports whether there's a Rex session to open in.
func Available() bool { return Session() != "" }

// Where rdxt opens.
type Where int

const (
	Float  Where = iota // a floating window over the current tab
	Split               // a pane beside the calling one
	Tab                 // a new tab
	Beside              // a pane beside the focused one, taking the focus
)

// Open runs `self args...` in Rex. For Split it goes beside $REX_BLOCK and
// leaves the focus where it was (so an agent's pane keeps it).
func Open(where Where, self string, args []string, dir, label string) error {
	session := Session()
	if session == "" {
		return errors.New("no Rex session found")
	}
	cmd := append([]string{self}, args...)
	switch where {
	case Split:
		a := []string{"split", "-s", session, "--focus=false"}
		if b := os.Getenv("REX_BLOCK"); b != "" {
			a = append(a, "-b", b)
		}
		if dir != "" {
			a = append(a, "-c", dir)
		}
		return run(append(append(a, "--"), cmd...)...)
	case Beside:
		a := []string{"split", "-s", session}
		if dir != "" {
			a = append(a, "-c", dir)
		}
		return run(append(append(a, "--"), cmd...)...)
	case Tab:
		a := []string{"run", "-s", session}
		if dir != "" {
			a = append(a, "-c", dir)
		}
		return run(append(append(a, "--"), cmd...)...)
	}
	opts := map[string]any{"command": cmd}
	if dir != "" {
		opts["cwd"] = dir
	}
	body, _ := json.Marshal(map[string]any{
		"bounds": []float64{0.1, 0.06, 0.8, 0.88},
		"focus":  true,
		"layout": map[string]any{"block": map[string]any{
			"flavor": "com.superlogical.terminal.shell", "label": label, "options": opts,
		}},
	})
	return run("api", "call", "-s", session, "session.new_layer", string(body))
}

func run(args ...string) error {
	out, err := exec.Command(Bin(), args...).CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(out))
		if msg == "" {
			msg = err.Error()
		}
		return errors.New(msg)
	}
	return nil
}
