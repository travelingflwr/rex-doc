package tui

import (
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/travelingflwr/rex-doc/internal/rex"
)

func pathUnescape(s string) (string, error) { return url.PathUnescape(s) }

// openExternal opens a URL or file with the system's default handler.
func openExternal(target string) error {
	name := "xdg-open"
	if runtime.GOOS == "darwin" {
		name = "open"
	}
	cmd := exec.Command(name, target)
	cmd.Stdout, cmd.Stderr = nil, nil
	return cmd.Start()
}

// runnable reports whether opening file with the system could run it: an
// app, a script, an installer, or anything marked executable.
func runnable(file string) bool {
	switch strings.ToLower(filepath.Ext(file)) {
	case ".app", ".command", ".tool", ".sh", ".bash", ".zsh", ".fish", ".csh", ".py", ".rb", ".pl",
		".js", ".ts", ".php", ".scpt", ".scptd", ".applescript", ".workflow", ".action", ".terminal",
		".pkg", ".mpkg", ".dmg", ".jar", ".exe", ".bat", ".cmd", ".ps1", ".url", ".webloc",
		".inetloc", ".fileloc", ".shortcut", ".prefpane", ".kext", ".plugin", ".bundle":
		return true
	}
	st, err := os.Stat(file)
	if err != nil {
		return true // can't tell: be careful
	}
	return !st.IsDir() && st.Mode()&0o111 != 0
}

// openOther opens a file that isn't Markdown. In Rex it opens in the user's
// editor in a split beside rdxt. Elsewhere the system opens it, but never
// anything it could run: a link in a document must not start a program.
func (v *viewer) openOther(file string) {
	if rex.Here() != "" {
		editor := os.Getenv("VISUAL")
		if editor == "" {
			editor = os.Getenv("EDITOR")
		}
		if editor == "" {
			editor = "vi"
		}
		cmd := exec.Command("rex", "split", "--", editor, file)
		if err := cmd.Run(); err == nil {
			v.setFlash("opened beside")
			return
		}
	}
	if runnable(file) {
		v.setFlash("won't open " + filepath.Base(file) + ": it could run a program")
		return
	}
	if err := openExternal(file); err != nil {
		v.setFlash("can't open " + file)
		return
	}
	v.setFlash("opened")
}
