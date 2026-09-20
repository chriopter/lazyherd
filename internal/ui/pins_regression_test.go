package ui

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestPinsSortedIsolatedAndRemovedOnReload(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "pins.json")
	p, err := loadPins(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range [][2]string{{"one", "z"}, {"two", "a"}, {"one", "a"}} {
		if err := p.toggle(entry[0], entry[1]); err != nil {
			t.Fatal(err)
		}
	}
	p, err = loadPins(path)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(p.byWorkspace, map[string][]string{"one": {"a", "z"}, "two": {"a"}}) {
		t.Fatal(p.byWorkspace)
	}
	if err := p.toggle("one", "a"); err != nil {
		t.Fatal(err)
	}
	if err := p.toggle("one", "z"); err != nil {
		t.Fatal(err)
	}
	p, err = loadPins(path)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(p.byWorkspace, map[string][]string{"two": {"a"}}) {
		t.Fatalf("empty workspace remains: %v", p.byWorkspace)
	}
	if _, err := os.Stat(path + ".tmp"); !os.IsNotExist(err) {
		t.Fatalf("temp file remains: %v", err)
	}
	raw, err := os.ReadFile(path)
	if err != nil || !strings.HasSuffix(string(raw), "\n") {
		t.Fatalf("saved file: %q %v", raw, err)
	}
}

func TestPinsReadAndSaveErrorsAreReported(t *testing.T) {
	dir := t.TempDir()
	if _, err := loadPins(dir); err == nil {
		t.Fatal("directory accepted as pins file")
	}
	blocker := filepath.Join(dir, "file")
	if err := os.WriteFile(blocker, []byte("block"), 0o644); err != nil {
		t.Fatal(err)
	}
	p := &pins{path: filepath.Join(blocker, "pins.json"), byWorkspace: map[string][]string{}}
	if err := p.toggle("ws", "repo"); err == nil {
		t.Fatal("save error swallowed")
	}
	m := newModel(dir, blocker)
	if !strings.HasPrefix(m.status, "pins: ") {
		t.Fatalf("load error not shown: %q", m.status)
	}
}
