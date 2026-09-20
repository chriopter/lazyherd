package ui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/chriopter/lazyherd/internal/repo"
)

// run drives a command through Update the way Bubble Tea would.
func run(t *testing.T, m Model, cmd tea.Cmd) Model {
	t.Helper()
	if cmd == nil {
		return m
	}
	msg := cmd()
	if batch, ok := msg.(tea.BatchMsg); ok {
		for _, c := range batch {
			m = run(t, m, c)
		}
		return m
	}
	next, _ := m.Update(msg)
	return next.(Model)
}

func TestInitialScanIsAccepted(t *testing.T) {
	t.Setenv("PATH", t.TempDir()) // no herdr
	root := t.TempDir()
	m := New(root)
	m = run(t, m, m.Init())
	if m.loading {
		t.Fatal("initial scan result was dropped")
	}
}

func TestStaleScanIsDropped(t *testing.T) {
	m := New(t.TempDir())
	m.setRepos([]repo.Repo{{Name: "keep"}})
	next, _ := m.Update(scanMsg{gen: m.gen - 1, repos: []repo.Repo{{Name: "old"}}})
	if got := next.(Model).current().Name; got != "keep" {
		t.Fatalf("stale scan applied: got %q", got)
	}
}

func TestSetReposKeepsSelectionByName(t *testing.T) {
	m := New(t.TempDir())
	m.setRepos([]repo.Repo{{Name: "a"}, {Name: "b"}, {Name: "c"}})
	m.cursor = 2 // "c"

	m.setRepos([]repo.Repo{{Name: "c"}, {Name: "a"}})
	if got := m.current().Name; got != "c" {
		t.Fatalf("selection lost after reorder: got %q", got)
	}

	m.setRepos([]repo.Repo{{Name: "a"}})
	if got := m.current().Name; got != "a" {
		t.Fatalf("selection after removal: got %q", got)
	}

	m.setRepos(nil)
	if m.current() != nil {
		t.Fatal("expected no selection for empty list")
	}
}

func TestFilterIsCaseInsensitive(t *testing.T) {
	m := New(t.TempDir())
	m.setRepos([]repo.Repo{{Name: "Alpha"}, {Name: "beta"}, {Name: "alphabet"}})
	m.filter = "ALPHA"
	m.refilter()
	if len(m.visible) != 2 {
		t.Fatalf("want 2 visible, got %d", len(m.visible))
	}
}

func TestViewSurvivesTinyTerminal(t *testing.T) {
	m := New(t.TempDir())
	m.setRepos([]repo.Repo{{Name: "a"}})
	for _, size := range [][2]int{{1, 1}, {10, 3}, {60, 8}, {200, 50}} {
		m.width, m.height = size[0], size[1]
		if m.View() == "" {
			t.Errorf("empty view at %v", size)
		}
	}
}
