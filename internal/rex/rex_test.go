package rex

import (
	"os"
	"strings"
	"testing"
)

func TestSessionFromRexPreferences(t *testing.T) {
	if _, err := os.Stat(os.ExpandEnv("$HOME/Library/Preferences/com.superlogical.rex.plist")); err != nil {
		t.Skip("Rex isn't installed here")
	}
	t.Setenv("REX_SESSION", "")
	t.Setenv("REX_BLOCK", "")
	if s := Session(); !strings.HasPrefix(s, "session:") {
		t.Fatalf("Session() = %q, want a session: ID from Rex's preferences", s)
	}
}

func TestHereUsesBlockWhenSessionIsUnset(t *testing.T) {
	t.Setenv("REX_SESSION", "")
	t.Setenv("REX_BLOCK", "block:abc")
	if got := Here(); got != "block:abc" {
		t.Fatalf("Here() = %q, want the block ID (new Rex servers don't set REX_SESSION)", got)
	}
	t.Setenv("REX_SESSION", "session:xyz")
	if got := Here(); got != "session:xyz" {
		t.Fatalf("Here() = %q, want REX_SESSION when an older server set it", got)
	}
	t.Setenv("REX_SESSION", "")
	t.Setenv("REX_BLOCK", "")
	if got := Here(); got != "" {
		t.Fatalf("Here() = %q outside Rex, want empty", got)
	}
}
