package tui

import "testing"

func TestEscClosesAFloatingWindow(t *testing.T) {
	v := &viewer{width: 60, height: 20, path: "x.md", sel: -1, floating: true}
	if !v.key("esc") {
		t.Error("Esc didn't close a floating window")
	}
	v.floating = false
	if v.key("esc") {
		t.Error("Esc closed a pane")
	}
}

func TestEscClearsBeforeClosing(t *testing.T) {
	v := &viewer{width: 60, height: 20, path: "x.md", sel: 2, floating: true}
	if v.key("esc") {
		t.Error("Esc closed the window instead of clearing the selection")
	}
	if !v.key("esc") {
		t.Error("a second Esc didn't close it")
	}
}

func TestDoubleEscFromSourceStays(t *testing.T) {
	v := &viewer{width: 60, height: 20, path: "x.md", sel: -1, floating: true, leftSource: true}
	if v.key("esc") {
		t.Error("the Esc right after leaving Source mode closed the window")
	}
	if !v.key("esc") {
		t.Error("the next Esc didn't close it")
	}
}
