package lazygit

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadOverridesDefaults(t *testing.T) {
	p := filepath.Join(t.TempDir(), "config.yml")
	os.WriteFile(p, []byte("gui:\n  border: single\n  nerdFontsVersion: \"3\"\n  theme:\n    activeBorderColor:\n      - '#ff00ff'\n      - bold\n"), 0o644)
	c := Load(p)
	if c.Gui.Border != "single" || c.Gui.NerdFontsVersion != "3" {
		t.Fatalf("overrides not applied: %+v", c.Gui)
	}
	if c.Gui.Theme.ActiveBorderColor[0] != "#ff00ff" || c.Gui.Theme.SelectedLineBgColor[0] != "blue" {
		t.Fatalf("theme merge wrong: %+v", c.Gui.Theme)
	}
	if IconsFor(c.Gui.NerdFontsVersion) == nil || IconsFor("") != nil {
		t.Fatal("icons should follow nerdFontsVersion")
	}
	if FrameRunes(c.Gui.Border)[2] != '┌' || FrameRunes("rounded")[2] != '╭' {
		t.Fatal("frame runes wrong")
	}
	if d := Load(filepath.Join(t.TempDir(), "missing.yml")); d.Gui.Border != "rounded" {
		t.Fatal("missing file should yield defaults")
	}
}
