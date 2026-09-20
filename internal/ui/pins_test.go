package ui

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPinsToggleAndReload(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", "pins.json")
	p, err := loadPins(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := p.toggle("backend", "api"); err != nil {
		t.Fatal(err)
	}
	if err := p.toggle("backend", "billing"); err != nil {
		t.Fatal(err)
	}
	if !p.has("backend", "api") || p.has("web", "api") {
		t.Fatal("pin state wrong")
	}

	again, err := loadPins(path)
	if err != nil {
		t.Fatal(err)
	}
	if !again.has("backend", "api") || !again.has("backend", "billing") {
		t.Fatal("pins not persisted")
	}
	if err := again.toggle("backend", "api"); err != nil {
		t.Fatal(err)
	}
	if again.has("backend", "api") {
		t.Fatal("unpin failed")
	}
}

func TestPinsLoadNullAndBroken(t *testing.T) {
	dir := t.TempDir()
	null := filepath.Join(dir, "null.json")
	if err := os.WriteFile(null, []byte("null\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	p, err := loadPins(null)
	if err != nil {
		t.Fatal(err)
	}
	if err := p.toggle("ws", "repo"); err != nil || !p.has("ws", "repo") {
		t.Fatal("toggle on a null file must work")
	}
	broken := filepath.Join(dir, "broken.json")
	if err := os.WriteFile(broken, []byte("{oops"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := loadPins(broken); err == nil {
		t.Fatal("broken JSON should be reported")
	}
}

func TestPluginDirectoriesWin(t *testing.T) {
	t.Setenv("HERDR_PLUGIN_CONFIG_DIR", "")
	t.Setenv("HERDR_PLUGIN_STATE_DIR", "")
	base, err := os.UserConfigDir()
	if err != nil {
		t.Fatal(err)
	}
	if ConfigDir() != filepath.Join(base, "lazyherd") || pinsPath() != filepath.Join(base, "lazyherd", "pins.json") {
		t.Fatalf("without the plugin: %s, %s", ConfigDir(), pinsPath())
	}
	t.Setenv("HERDR_PLUGIN_CONFIG_DIR", "/cfg")
	t.Setenv("HERDR_PLUGIN_STATE_DIR", "/state")
	if ConfigDir() != "/cfg" || pinsPath() != "/state/pins.json" {
		t.Fatalf("as a plugin: %s, %s", ConfigDir(), pinsPath())
	}
}
