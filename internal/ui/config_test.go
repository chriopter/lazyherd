package ui

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadConfigOverridesDefaults(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yml")
	if err := os.WriteFile(path, []byte("gui:\n  border: single\n  nerdFontsVersion: \"3\"\n  theme:\n    activeBorderColor:\n      - '#ff00ff'\n      - bold\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	c := loadConfig(path)
	if c.Gui.Border != "single" || c.Gui.NerdFontsVersion != "3" {
		t.Fatalf("overrides not applied: %+v", c.Gui)
	}
	if c.Gui.Theme.ActiveBorderColor[0] != "#ff00ff" || c.Gui.Theme.SelectedLineBgColor[0] != "blue" {
		t.Fatalf("theme merge wrong: %+v", c.Gui.Theme)
	}
	if configIcons(c.Gui.NerdFontsVersion) == nil || configIcons("") != nil {
		t.Fatal("icons should follow nerdFontsVersion")
	}
	if frameRunes(c.Gui.Border)[2] != '┌' || frameRunes("rounded")[2] != '╭' {
		t.Fatal("frame runes wrong")
	}
	if defaults := loadConfig(filepath.Join(t.TempDir(), "missing.yml")); defaults.Gui.Border != "rounded" {
		t.Fatal("missing file should yield defaults")
	}
}
