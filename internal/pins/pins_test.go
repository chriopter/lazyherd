package pins

import (
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
