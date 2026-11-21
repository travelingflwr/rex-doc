package rex

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLineIn(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "init.lua")
	os.WriteFile(cfg, nil, 0o644)
	os.MkdirAll(filepath.Join(dir, "plugins", "x"), 0o755)
	os.WriteFile(filepath.Join(dir, "plugins", "x", "init.lua"), nil, 0o644)
	for src, want := range map[string]int{
		"init.lua:3":           3,
		cfg + ":12":            12,
		"plugins/x/init.lua:4": 0,
		"init.lua":             0,
		"init.lua:x":           0,
	} {
		if got := lineIn(src, cfg); got != want {
			t.Errorf("lineIn(%q) = %d, want %d", src, got, want)
		}
	}
}

func TestOnlyRexsConfigIsChecked(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "rex", "init.lua")
	os.MkdirAll(filepath.Dir(cfg), 0o755)
	os.WriteFile(cfg, nil, 0o644)
	other := filepath.Join(dir, "repo", "init.lua")
	os.MkdirAll(filepath.Dir(other), 0o755)
	os.WriteFile(other, nil, 0o644)
	t.Setenv("REX_CONFIG", cfg)
	if !IsConfig(cfg) {
		t.Error("Rex's config isn't recognized")
	}
	if IsConfig(other) {
		t.Error("another init.lua was taken for Rex's config")
	}
}
