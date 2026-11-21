// rdxt: Markdown, made for Rex.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"golang.org/x/term"

	"github.com/travelingflwr/rex-doc/internal/doc"
	"github.com/travelingflwr/rex-doc/internal/rex"
	"github.com/travelingflwr/rex-doc/internal/setup"
	"github.com/travelingflwr/rex-doc/internal/tui"
)

var version = "dev"

const usage = `rdxt: Markdown, made for Rex.

Usage:
  rdxt FILE                  open FILE here
  rdxt open [--float|--split|--tab] FILE
                             open FILE in Rex: beside you (the default in a
                             Rex pane), floating, or in a new tab
  rdxt new [--template T] NAME
                             create a document and open it ready to edit;
                             templates: blank, plan, notes, dok
  rdxt -p FILE               print FILE laid out and exit (also when piped)
  rdxt setup [--undo]        make rdxt feel built in: Cmd-click and Finder
                             open .md and .dok in rdxt, Rex palette actions,
                             agent skills, shell setup (--undo removes it all)
  rdxt setup --open pane|float
                             where clicked documents open: a pane beside
                             the focused one, or a floating window (default)
  rdxt setup --txt[=false]   open .txt files in rdxt too (or stop)
  rdxt ~/.config/rex/init.lua
                             Rex's config, with Rex's check of it on top
  rdxt agents install|remove add or remove the agent skill only
  rdxt agent-guide           how coding agents should use rdxt

Options:
  -p, --print         print instead of opening full screen
  -w, --width N       layout width for --print (default: the terminal's, or 80)
      --no-color      print without colors (also when NO_COLOR is set)
      --source        start in Source mode, ready to edit
  -v, --version       print the version

Keys (press ? in rdxt for all of them):
  j/k  ↓/↑  Space/b  scroll                e  Esc            Source mode, back to View
  Tab  Enter  f      links                 H  L              back, forward
  /  n  N            search                o                 outline
  q                  quit

★ Like rdxt? A star helps others find it: https://github.com/travelingflwr/rex-doc
`

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "open":
			exit(openCmd(os.Args[2:]))
		case "new":
			exit(newCmd(os.Args[2:]))
		case "list":
			exit(listCmd(os.Args[2:]))
		case "setup":
			exit(setup.Command(os.Args[2:], version))
		case "agents":
			exit(setup.AgentsCommand(os.Args[2:]))
		case "agent-guide":
			fmt.Print(setup.AgentGuide)
			return
		case "help":
			fmt.Print(usage)
			return
		}
	}
	fs := flag.NewFlagSet("rdxt", flag.ContinueOnError)
	fs.Usage = func() { fmt.Fprint(os.Stderr, usage) }
	var print, noColor, showVersion, source bool
	var width int
	var imagePath string
	fs.BoolVar(&print, "p", false, "")
	fs.BoolVar(&print, "print", false, "")
	fs.IntVar(&width, "w", 0, "")
	fs.IntVar(&width, "width", 0, "")
	fs.BoolVar(&noColor, "no-color", false, "")
	fs.BoolVar(&source, "source", false, "")
	fs.BoolVar(&showVersion, "v", false, "")
	fs.BoolVar(&showVersion, "version", false, "")
	fs.StringVar(&imagePath, "image", "", "")
	if err := fs.Parse(os.Args[1:]); err != nil {
		if err == flag.ErrHelp {
			return
		}
		os.Exit(2)
	}
	if showVersion {
		fmt.Println("rdxt", version)
		fmt.Println("★ Like it? Star it: https://github.com/travelingflwr/rex-doc")
		return
	}
	if imagePath != "" {
		exit(tui.RunImage(imagePath))
	}
	if fs.NArg() != 1 {
		fs.Usage()
		os.Exit(2)
	}
	path := fs.Arg(0)
	if print || !term.IsTerminal(int(os.Stdout.Fd())) || path == "-" {
		if os.Getenv("NO_COLOR") != "" {
			noColor = true
		}
		exit(printDoc(path, width, !noColor))
	}
	exit(tui.RunWith(path, tui.RunOptions{Source: source}))
}

func exit(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, "rdxt:", err)
		os.Exit(1)
	}
	os.Exit(0)
}

// self is how other programs should run this rdxt: a stable path when
// there is one (a Homebrew or ~/.local/bin link), not a versioned one.
func self() string {
	if p, err := os.Executable(); err == nil {
		if lp, err := exec.LookPath("rdxt"); err == nil {
			if a, _ := filepath.EvalSymlinks(lp); a != "" {
				if b, _ := filepath.EvalSymlinks(p); a == b {
					return lp
				}
			}
		}
		return p
	}
	return "rdxt"
}

func openCmd(args []string) error {
	fs := flag.NewFlagSet("rdxt open", flag.ContinueOnError)
	float := fs.Bool("float", false, "")
	split := fs.Bool("split", false, "")
	tab := fs.Bool("tab", false, "")
	// --click is a Cmd-click or a Finder double-click, from rdxt's
	// document handler: it opens where `rdxt setup --open` said.
	click := fs.Bool("click", false, "")
	source := fs.Bool("source", false, "")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return fmt.Errorf("usage: rdxt open [--float|--split|--tab] [--source] FILE")
	}
	path, err := filepath.Abs(fs.Arg(0))
	if err != nil {
		return err
	}
	if _, err := os.Stat(path); err != nil {
		return err
	}
	where := rex.Float
	switch {
	case *split:
		where = rex.Split
	case *tab:
		where = rex.Tab
	case *float:
	case *click:
		if setup.OpenIn() == "pane" {
			where = rex.Beside
		}
	case os.Getenv("REX_BLOCK") != "":
		where = rex.Split // from a Rex pane (an agent, a shell): beside it
	}
	run := []string{path}
	if *source {
		run = []string{"--source", path}
	}
	if !rex.Available() {
		// No Rex: open right here when there's a terminal.
		if term.IsTerminal(int(os.Stdout.Fd())) {
			return tui.RunWith(path, tui.RunOptions{Source: *source})
		}
		return fmt.Errorf("Rex isn't running, and there's no terminal to open %s in", filepath.Base(path))
	}
	if err := rex.Open(where, self(), run, filepath.Dir(path), filepath.Base(path)); err != nil {
		return err
	}
	fmt.Println(path)
	return nil
}

func newCmd(args []string) error {
	fs := flag.NewFlagSet("rdxt new", flag.ContinueOnError)
	template := fs.String("template", "blank", "")
	dir := fs.String("dir", ".", "")
	noOpen := fs.Bool("no-open", false, "")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return fmt.Errorf("usage: rdxt new [--template blank|plan|notes|dok] [--dir DIR] NAME")
	}
	path, err := setup.NewDocument(*dir, fs.Arg(0), *template)
	if err != nil {
		return err
	}
	fmt.Println(path)
	if *noOpen || !term.IsTerminal(int(os.Stdout.Fd())) {
		return nil
	}
	return tui.RunWith(path, tui.RunOptions{Source: true})
}

// listCmd prints documents for the Rex palette's Open Document: recent
// ones first, then the ones in the folder.
func listCmd(args []string) error {
	fs := flag.NewFlagSet("rdxt list", flag.ContinueOnError)
	asJSON := fs.Bool("json", false, "")
	if err := fs.Parse(args); err != nil {
		return err
	}
	dir := "."
	if fs.NArg() > 0 {
		dir = fs.Arg(0)
	}
	type item struct {
		Path   string `json:"path"`
		Name   string `json:"name"`
		Dir    string `json:"dir"`
		Recent bool   `json:"recent"`
	}
	home, _ := os.UserHomeDir()
	short := func(p string) string {
		if home != "" && strings.HasPrefix(p, home) {
			return "~" + p[len(home):]
		}
		return p
	}
	var items []item
	seen := map[string]bool{}
	for _, p := range tui.Recent() {
		if len(items) >= 15 {
			break
		}
		seen[p] = true
		items = append(items, item{p, filepath.Base(p), short(filepath.Dir(p)), true})
	}
	abs, _ := filepath.Abs(dir)
	for _, p := range setup.FindDocuments(abs, 300) {
		if !seen[p] {
			items = append(items, item{p, filepath.Base(p), short(filepath.Dir(p)), false})
		}
	}
	if *asJSON {
		return json.NewEncoder(os.Stdout).Encode(items)
	}
	for _, it := range items {
		fmt.Println(it.Path)
	}
	return nil
}

func printDoc(path string, width int, color bool) error {
	var src []byte
	var err error
	if path == "-" {
		src, err = io.ReadAll(os.Stdin)
	} else {
		src, err = os.ReadFile(path)
	}
	if err != nil {
		return err
	}
	if width <= 0 {
		if w, _, err := term.GetSize(int(os.Stdout.Fd())); err == nil && w > 0 {
			width = w
		} else if c, err := strconv.Atoi(os.Getenv("COLUMNS")); err == nil && c > 0 {
			width = c
		} else {
			width = 80
		}
	}
	opts := doc.OptionsFor(path)
	opts.ShowFrontmatter = true
	if path != "-" && rex.IsConfig(path) {
		opts.Check = rex.CheckConfig()
	}
	d := doc.Render(src, width, opts)
	for i := range d.Lines {
		fmt.Println(d.Format(i, color))
	}
	return nil
}
