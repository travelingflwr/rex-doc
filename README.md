# rex-doc

[![GitHub stars](https://img.shields.io/github/stars/travelingflwr/rex-doc?style=social)](https://github.com/travelingflwr/rex-doc/stargazers)

## Markdown, made for [Rex](https://superlogical.com).

Read and write Markdown as real text: fast, with links you can follow, at home in your terminal. rdxt is a terminal app (a TUI), not a separate window: it runs in a Rex pane, a floating Rex window or any terminal, and hooks into Rex's palette, Cmd-click and your coding agents. It works in any terminal, but it's designed around Rex: its panes, its floating windows and its palette.

`rdxt` ("read text") opens a Markdown file as real terminal text, not a picture. So it uses your terminal's font, size and theme, zooms with ⌘+, and works with selection and scrolling, including on the iPad.

## Install

```sh
brew install travelingflwr/tap/rdxt
rdxt setup
```

Or download a build for macOS or Linux from [Releases](https://github.com/travelingflwr/rex-doc/releases), or build it yourself with `go install github.com/travelingflwr/rex-doc/cmd/rdxt@latest`.

`rdxt setup` makes rdxt feel built into Rex:
- **Cmd-click** a `.md` or `.dok` path in Rex, or double-click one in Finder, and it opens in a floating rdxt window. `Esc` closes it. To read beside an agent instead, run `rdxt setup --open pane`, and clicked documents open in a pane next to the one you clicked in.
- **The Rex palette** gets *rdxt: Open Document…*, *rdxt: New Document…* (with Blank, Plan, Notes and Dok templates) and *rdxt: Check Rex Config*.
- **Coding agents** (Claude Code, Codex, Cursor, OpenCode, Pi) get an rdxt skill, so they open the plans and reports they write.
- **zsh** reports its folder to Rex, so relative names are clickable, and typing `notes.md` + Enter opens it.

- **Plain text, if you like:** `rdxt setup --txt` makes rdxt open `.txt` files too. `--txt=false` gives them back to the app they had.

`rdxt setup --undo` puts everything back, including the apps that opened Markdown and text before.

**On a remote host:** Cmd-click a document in a pane on another Rex host, and Rex downloads a copy and opens it in rdxt. To read it live, with links to other documents working, install rdxt on that host too and run `rdxt notes.md` there.

## Use

```sh
rdxt notes.md                    # read it here
rdxt open --float notes.md       # in a floating Rex window (--split, --tab)
rdxt new --template plan fix-login
```

Press `?` in rdxt for every key.

![rdxt showing an agent's plan in Rex: the top bar, a capitalized title, an alert and ticked tasks](docs/screenshots/view.png)

| Mermaid sequence diagrams | Mermaid flowcharts |
|---|---|
| ![A Mermaid sequence diagram drawn as text: participants, lifelines, numbered messages and a note](docs/screenshots/sequence.png) | ![A Mermaid flowchart drawn as text, with a decision node and labeled branches](docs/screenshots/flowchart.png) |

| Source mode (`e`) | Search (`/`) | Outline (`o`) |
|---|---|---|
| ![Source mode with syntax colors and line numbers](docs/screenshots/source.png) | ![Search with every match highlighted](docs/screenshots/search.png) | ![The outline of a document](docs/screenshots/outline.png) |

## The idea

- **View mode** (the default): the rendered document. Links to other `.md`, `.dok` and `.txt` files open in place, with Back and Forward.
- **Source mode** (`e`): the raw Markdown, always editable, with vim-like keys. `Esc` goes back to View.
- **Built for reviewing what agents write:** it opens fast and starts in View mode.
- **Adapts to the pane:** compact in narrow panes (phone, iPad split view), with an outline sidebar in wide ones.
- **Supports** CommonMark, GFM, GitHub alerts, frontmatter, math and `:::` directives, and HTML `<img>` tags.
- **Plain text too:** `.txt` files are shown as written, with columns kept and long lines wrapped.
- **Your Rex config, checked:** open `~/.config/rex/init.lua` in rdxt (or *rdxt: Check Rex Config* in the palette), and Rex's own check of it comes first: what it sets up, or each error with its line, and those lines numbered in red. Edit it with `e`; saving checks it again. Other `.lua` files get syntax colors and line numbers, and are never run.
- **Mermaid diagrams drawn as text:** sequence diagrams, flowcharts (TD, LR, BT, RL) and pie charts, in your theme's colors. Other kinds show as a picture when `mmdc` is installed, or as their source.

## ⭐ Like it? Star it

If rdxt makes reading Markdown in your terminal better, **please star the repo**. Stars are how other people find tools like this, and they tell us it's worth building more.

## Contributing

rdxt is deliberately opinionated. Please read [CONTRIBUTING.md](CONTRIBUTING.md) before sending a pull request. It explains the design decisions and why pull requests that go against them are declined.

## License

MIT
