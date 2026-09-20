package pins

import (
	"os"
	"path/filepath"
	"testing"
)

func TestToggleAndReload(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", "pins.json")
	s, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Toggle("backend", "api"); err != nil {
		t.Fatal(err)
	}
	if err := s.Toggle("backend", "billing"); err != nil {
		t.Fatal(err)
	}
	if !s.Pinned("backend", "api") || s.Pinned("web", "api") {
		t.Fatal("pin state wrong")
	}

	again, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if !again.Pinned("backend", "api") || !again.Pinned("backend", "billing") {
		t.Fatal("pins not persisted")
	}
	if err := again.Toggle("backend", "api"); err != nil {
		t.Fatal(err)
	}
	if again.Pinned("backend", "api") {
		t.Fatal("unpin failed")
	}
}

func TestLoadNullAndBroken(t *testing.T) {
	dir := t.TempDir()
	null := filepath.Join(dir, "null.json")
	os.WriteFile(null, []byte("null\n"), 0o644)
	s, err := Load(null)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Toggle("ws", "repo"); err != nil || !s.Pinned("ws", "repo") {
		t.Fatal("toggle on a null file must work")
	}
	broken := filepath.Join(dir, "broken.json")
	os.WriteFile(broken, []byte("{oops"), 0o644)
	if _, err := Load(broken); err == nil {
		t.Fatal("broken JSON should be reported")
	}
}
