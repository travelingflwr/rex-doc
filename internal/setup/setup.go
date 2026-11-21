// Package setup makes rdxt feel built in: the default app for .md and .dok
// (Cmd-click in Rex, double-click in Finder), Rex palette actions, an agent
// skill for every coding agent, and shell setup. Everything it changes is
// recorded so `rdxt setup --undo` can put it back.
package setup

import (
	_ "embed"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/travelingflwr/rex-doc/internal/rex"
)

var (
	//go:embed assets/handler.swift
	handlerSwift string
	//go:embed assets/Info.plist
	infoPlist string
	//go:embed assets/plugin.lua
	pluginLua string
	//go:embed assets/skill.md
	skillMD string
	//go:embed assets/zsh.sh
	zshBlock string
	//go:embed assets/guide.md
	AgentGuide string
)

const (
	markStart  = "-- >>> rdxt (managed by rdxt setup; rdxt setup --undo removes it) >>>"
	markEnd    = "-- <<< rdxt <<<"
	zshStart   = "# >>> rdxt (managed by rdxt setup; rdxt setup --undo removes it) >>>"
	zshEnd     = "# <<< rdxt <<<"
	lsregister = "/System/Library/Frameworks/CoreServices.framework/Frameworks/LaunchServices.framework/Support/lsregister"
)

// state is what setup changed, for --undo.
type state struct {
	Version  string            `json:"version"`
	When     time.Time         `json:"when"`
	App      string            `json:"app,omitempty"`
	Previous map[string]string `json:"previous_handlers,omitempty"`
	Plugin   string            `json:"plugin,omitempty"`
	Skills   []string          `json:"skills,omitempty"`
	Zshrc    string            `json:"zshrc,omitempty"`
	// Open is where clicked documents open: "float" (the default) or "pane".
	Open string `json:"open,omitempty"`
}

// OpenIn is where Cmd-click, a Finder double-click and the palette open a
// document: "float" for a floating window, or "pane" for a pane beside
// the focused one. `rdxt setup --open` sets it.
func OpenIn() string { return loadState().openIn() }

func (s state) openIn() string {
	if s.Open == "pane" {
		return "pane"
	}
	return "float"
}

func home() string { h, _ := os.UserHomeDir(); return h }

func stateFile() string { return filepath.Join(home(), ".local", "state", "rdxt", "setup.json") }

func loadState() state {
	var s state
	if b, err := os.ReadFile(stateFile()); err == nil {
		json.Unmarshal(b, &s)
	}
	return s
}

func saveState(s state) error {
	if err := os.MkdirAll(filepath.Dir(stateFile()), 0o755); err != nil {
		return err
	}
	b, _ := json.MarshalIndent(s, "", "  ")
	return os.WriteFile(stateFile(), b, 0o644)
}

// step prints one line of progress.
func step(ok bool, what string, detail string) {
	mark := "\x1b[32m✔\x1b[0m"
	if !ok {
		mark = "\x1b[33m–\x1b[0m"
	}
	if detail != "" {
		detail = " \x1b[90m" + detail + "\x1b[0m"
	}
	fmt.Printf("  %s %s%s\n", mark, what, detail)
}

// Command runs `rdxt setup [--undo]`.
func Command(args []string, version string) error {
	fs := flag.NewFlagSet("rdxt setup", flag.ContinueOnError)
	undo := fs.Bool("undo", false, "")
	open := fs.String("open", "", "")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *undo {
		return Undo()
	}
	switch *open {
	case "", "float", "pane":
	default:
		return fmt.Errorf("--open is float or pane, not %q", *open)
	}
	return Install(version, *open)
}

// self is the stable path to this rdxt (a Homebrew or ~/.local/bin link
// rather than a versioned file), so upgrades keep working.
func self() string {
	exe, err := os.Executable()
	if err != nil {
		return "rdxt"
	}
	if lp, err := exec.LookPath("rdxt"); err == nil {
		a, _ := filepath.EvalSymlinks(lp)
		b, _ := filepath.EvalSymlinks(exe)
		if a != "" && a == b {
			if abs, err := filepath.Abs(lp); err == nil {
				return abs
			}
		}
	}
	return exe
}

// Install sets everything up. Each part is independent: one failing
// doesn't stop the others.
// open, when not empty, changes where clicked documents open.
func Install(version, open string) error {
	fmt.Println("\x1b[1mSetting up rdxt\x1b[0m")
	s := loadState()
	s.Version, s.When = version, time.Now()
	if open != "" {
		s.Open = open
	}
	where := "floating window in Rex"
	if s.openIn() == "pane" {
		where = "pane beside the focused one in Rex"
	}
	bin := self()
	var problems []string

	if runtime.GOOS == "darwin" {
		if err := installHandler(&s, bin, version); err != nil {
			step(false, "Cmd-click and Finder open documents in rdxt", err.Error())
			problems = append(problems, "handler")
		} else {
			step(true, "Cmd-click and Finder open .md and .dok in rdxt", "default app, "+where)
		}
	}
	if err := installPlugin(&s, bin, s.openIn()); err != nil {
		step(false, "Rex palette actions", err.Error())
		problems = append(problems, "palette")
	} else {
		step(true, "Rex palette: rdxt: Open Document… and rdxt: New Document…", where)
	}
	if dirs := installSkills(&s); len(dirs) > 0 {
		step(true, "Agent skill for "+strings.Join(dirs, ", "), "")
	} else {
		step(false, "Agent skill", "no coding agents found (~/.claude, ~/.agents, ~/.cursor)")
	}
	if err := installZsh(&s); err != nil {
		step(false, "Shell setup", err.Error())
	} else if s.Zshrc != "" {
		step(true, "zsh: clickable relative names in Rex, and notes.md + Enter opens it", "new shells pick it up")
	}
	if err := saveState(s); err != nil {
		return err
	}
	if tip := ezaTip(); tip != "" {
		fmt.Println("\n  \x1b[1mTip:\x1b[0m " + tip)
	}
	fmt.Println()
	fmt.Println("Done. Cmd-click a .md file in Rex, or open the palette and type rdxt.")
	fmt.Println("\x1b[90mrdxt setup --undo puts everything back.\x1b[0m")
	fmt.Println()
	fmt.Println("★ Like rdxt? A star helps others find it: https://github.com/travelingflwr/rex-doc")
	if len(problems) > 0 {
		return fmt.Errorf("some parts weren't set up: %s", strings.Join(problems, ", "))
	}
	return nil
}

// ---- The document handler (macOS) ----

func installHandler(s *state, bin, version string) error {
	swiftc, err := exec.LookPath("swiftc")
	if err != nil {
		return errors.New("needs the Xcode command line tools (xcode-select --install)")
	}
	app := filepath.Join(home(), "Applications", "rdxt.app")
	build, err := os.MkdirTemp("", "rdxt-handler")
	if err != nil {
		return err
	}
	defer os.RemoveAll(build)
	src := strings.NewReplacer("__RDXT__", bin, "__REX__", rex.Bin()).Replace(handlerSwift)
	if err := os.WriteFile(filepath.Join(build, "main.swift"), []byte(src), 0o644); err != nil {
		return err
	}
	exe := filepath.Join(build, "rdxt-handler")
	if out, err := exec.Command(swiftc, "-O", "-suppress-warnings", "-o", exe, filepath.Join(build, "main.swift")).CombinedOutput(); err != nil {
		return fmt.Errorf("couldn't build the handler: %s", strings.TrimSpace(string(out)))
	}
	if err := os.RemoveAll(app); err != nil {
		return err
	}
	macos := filepath.Join(app, "Contents", "MacOS")
	if err := os.MkdirAll(macos, 0o755); err != nil {
		return err
	}
	if err := copyFile(exe, filepath.Join(macos, "rdxt-handler"), 0o755); err != nil {
		return err
	}
	plist := strings.ReplaceAll(infoPlist, "__VERSION__", strings.TrimPrefix(version, "v"))
	if err := os.WriteFile(filepath.Join(app, "Contents", "Info.plist"), []byte(plist), 0o644); err != nil {
		return err
	}
	exec.Command("codesign", "--force", "--sign", "-", app).Run()
	if out, err := exec.Command(lsregister, "-f", app).CombinedOutput(); err != nil {
		return fmt.Errorf("couldn't register the app: %s", strings.TrimSpace(string(out)))
	}
	out, err := exec.Command(filepath.Join(macos, "rdxt-handler"), "--set-default").Output()
	if err != nil {
		return fmt.Errorf("couldn't make rdxt the default app: %v", err)
	}
	prev := map[string]string{}
	json.Unmarshal(out, &prev)
	// Keep the first handlers we replaced, across repeated setups.
	if s.Previous == nil {
		s.Previous = prev
	}
	s.App = app
	return nil
}

func copyFile(from, to string, mode os.FileMode) error {
	b, err := os.ReadFile(from)
	if err != nil {
		return err
	}
	return os.WriteFile(to, b, mode)
}

// ---- Rex palette actions ----

func rexConfig() string {
	if c := os.Getenv("REX_CONFIG"); c != "" {
		return c
	}
	dir := os.Getenv("XDG_CONFIG_HOME")
	if dir == "" {
		dir = filepath.Join(home(), ".config")
	}
	return filepath.Join(dir, "rex", "init.lua")
}

func installPlugin(s *state, bin, open string) error {
	cfg := rexConfig()
	plugin := filepath.Join(filepath.Dir(cfg), "plugins", "rdxt", "init.lua")
	if err := os.MkdirAll(filepath.Dir(plugin), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(plugin, []byte(pluginLua), 0o644); err != nil {
		return err
	}
	block := fmt.Sprintf("%s\ndofile(%q).setup({ bin = %q, open = %q })\n%s\n", markStart, plugin, bin, open, markEnd)
	old, err := os.ReadFile(cfg)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	text := removeBlock(string(old), markStart, markEnd)
	if text != "" && !strings.HasSuffix(text, "\n") {
		text += "\n"
	}
	text += "\n" + block
	if err := writeChecked(cfg, old, text); err != nil {
		return err
	}
	s.Plugin = plugin
	return nil
}

// writeChecked writes Rex's config, checks it with `rex config check`,
// and puts the old one back if Rex doesn't accept it. Then it reloads.
func writeChecked(cfg string, old []byte, text string) error {
	if len(old) > 0 {
		os.WriteFile(cfg+".before-rdxt", old, 0o644)
	}
	if err := os.MkdirAll(filepath.Dir(cfg), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(cfg, []byte(text), 0o644); err != nil {
		return err
	}
	if out, err := exec.Command(rex.Bin(), "config", "check").CombinedOutput(); err != nil {
		os.WriteFile(cfg, old, 0o644)
		return fmt.Errorf("Rex didn't accept the config, so it was left as it was: %s", lastLine(string(out)))
	}
	exec.Command(rex.Bin(), "config", "reload").Run()
	return nil
}

func lastLine(s string) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	return lines[len(lines)-1]
}

// removeBlock cuts the text between start and end markers (inclusive),
// and the blank line before it.
func removeBlock(text, start, end string) string {
	i := strings.Index(text, start)
	if i < 0 {
		return text
	}
	j := strings.Index(text[i:], end)
	if j < 0 {
		return text
	}
	k := i + j + len(end)
	if k < len(text) && text[k] == '\n' {
		k++
	}
	before := strings.TrimRight(text[:i], "\n")
	if before != "" {
		before += "\n"
	}
	return before + text[k:]
}

// ---- Agent skills ----

// agentDirs are where coding agents look for skills, by the agent's own
// folder. A skill is only added for agents that are installed.
var agentDirs = []struct{ root, skills, name string }{
	{".claude", ".claude/skills", "Claude Code"},
	{".agents", ".agents/skills", "Codex, Pi, OpenCode"},
	{".codex", ".agents/skills", "Codex"},
	{".cursor", ".cursor/skills", "Cursor"},
}

func installSkills(s *state) []string {
	var names []string
	done := map[string]bool{}
	for _, a := range agentDirs {
		if _, err := os.Stat(filepath.Join(home(), a.root)); err != nil {
			continue
		}
		dir := filepath.Join(home(), a.skills, "rdxt")
		if done[dir] {
			continue
		}
		if os.MkdirAll(dir, 0o755) != nil || os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(skillMD), 0o644) != nil {
			continue
		}
		done[dir] = true
		names = append(names, a.name)
		if !contains(s.Skills, dir) {
			s.Skills = append(s.Skills, dir)
		}
	}
	return names
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

// AgentsCommand runs `rdxt agents install|remove`.
func AgentsCommand(args []string) error {
	if len(args) != 1 || (args[0] != "install" && args[0] != "remove") {
		return errors.New("usage: rdxt agents install|remove")
	}
	s := loadState()
	if args[0] == "install" {
		names := installSkills(&s)
		if len(names) == 0 {
			return errors.New("no coding agents found (~/.claude, ~/.agents, ~/.cursor)")
		}
		step(true, "Agent skill for "+strings.Join(names, ", "), "")
		return saveState(s)
	}
	removeSkills(&s)
	return saveState(s)
}

func removeSkills(s *state) {
	for _, dir := range s.Skills {
		if os.RemoveAll(dir) == nil {
			step(true, "Removed the agent skill", dir)
		}
	}
	s.Skills = nil
}

// ---- zsh ----

func installZsh(s *state) error {
	if !strings.HasSuffix(os.Getenv("SHELL"), "zsh") {
		return nil
	}
	rc := filepath.Join(zdotdir(), ".zshrc")
	old, err := os.ReadFile(rc)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	text := removeBlock(string(old), zshStart, zshEnd)
	if text != "" && !strings.HasSuffix(text, "\n") {
		text += "\n"
	}
	text += "\n" + zshStart + "\n" + zshBlock + zshEnd + "\n"
	if len(old) > 0 {
		os.WriteFile(rc+".before-rdxt", old, 0o644)
	}
	if err := os.WriteFile(rc, []byte(text), 0o644); err != nil {
		return err
	}
	s.Zshrc = rc
	return nil
}

// ezaTip suggests --hyperlink when ls is eza without it: then every name
// in a listing is a file link, and Cmd-click opens documents in rdxt.
func ezaTip() string {
	out, err := exec.Command("zsh", "-ic", "alias ls").Output()
	if err != nil {
		return ""
	}
	a := string(out)
	if strings.Contains(a, "eza") && !strings.Contains(a, "--hyperlink") {
		return "add --hyperlink to your eza aliases (ls, ll, la) so every file name is clickable."
	}
	return ""
}

// zdotdir is the folder zsh reads .zshrc from: $ZDOTDIR as zsh sets it
// (often in ~/.zshenv), or the home folder.
func zdotdir() string {
	if out, err := exec.Command("zsh", "-c", `print -r -- "${ZDOTDIR:-$HOME}"`).Output(); err == nil {
		if d := strings.TrimSpace(string(out)); d != "" {
			return d
		}
	}
	if d := os.Getenv("ZDOTDIR"); d != "" {
		return d
	}
	return home()
}

// ---- Undo ----

// Undo removes everything setup added and restores the apps that opened
// documents before.
func Undo() error {
	fmt.Println("\x1b[1mRemoving rdxt's setup\x1b[0m")
	s := loadState()
	if s.App != "" {
		var pairs []string
		for t, h := range s.Previous {
			pairs = append(pairs, t+"="+h)
		}
		handler := filepath.Join(s.App, "Contents", "MacOS", "rdxt-handler")
		if len(pairs) > 0 {
			exec.Command(handler, append([]string{"--restore"}, pairs...)...).Run()
		}
		exec.Command(lsregister, "-u", s.App).Run()
		trash := filepath.Join(home(), ".Trash", "rdxt.app")
		os.RemoveAll(trash)
		if err := os.Rename(s.App, trash); err != nil {
			os.RemoveAll(s.App)
		}
		step(true, "Removed the rdxt app and gave documents back to their previous apps", "")
		s.App, s.Previous = "", nil
	}
	if s.Plugin != "" {
		cfg := rexConfig()
		if old, err := os.ReadFile(cfg); err == nil {
			if err := writeChecked(cfg, old, removeBlock(string(old), markStart, markEnd)); err != nil {
				step(false, "Rex palette actions", err.Error())
			}
		}
		os.RemoveAll(filepath.Dir(s.Plugin))
		step(true, "Removed the Rex palette actions", "")
		s.Plugin = ""
	}
	removeSkills(&s)
	if s.Zshrc != "" {
		if old, err := os.ReadFile(s.Zshrc); err == nil {
			os.WriteFile(s.Zshrc, []byte(removeBlock(string(old), zshStart, zshEnd)), 0o644)
		}
		step(true, "Removed the zsh setup", "")
		s.Zshrc = ""
	}
	os.Remove(stateFile())
	fmt.Println("\nDone. rdxt itself is still installed; remove it with your package manager.")
	return nil
}
