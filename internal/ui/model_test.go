package ui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/chriopter/lazyherd/internal/repo"
)

// newTestModel builds a model outside of any Herdr workspace.
func newTestModel(t *testing.T) Model {
	t.Helper()
	t.Setenv("HERDR_WORKSPACE_ID", "")
	return New(t.TempDir())
}

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
	m := newTestModel(t)
	m = run(t, m, m.Init())
	if m.loading {
		t.Fatal("initial scan result was dropped")
	}
}

func TestStaleScanIsDropped(t *testing.T) {
	m := newTestModel(t)
	m.setRepos([]repo.Repo{{Name: "keep"}})
	next, _ := m.Update(scanMsg{gen: m.gen - 1, repos: []repo.Repo{{Name: "old"}}})
	if got := next.(Model).current().Name; got != "keep" {
		t.Fatalf("stale scan applied: got %q", got)
	}
}

func TestSetReposKeepsSelectionByName(t *testing.T) {
	m := newTestModel(t)
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
	m := newTestModel(t)
	m.setRepos([]repo.Repo{{Name: "Alpha"}, {Name: "beta"}, {Name: "alphabet"}})
	m.filter = "ALPHA"
	m.refilter()
	if len(m.visible) != 2 {
		t.Fatalf("want 2 visible, got %d", len(m.visible))
	}
}

func TestViewSurvivesTinyTerminal(t *testing.T) {
	m := newTestModel(t)
	m.setRepos([]repo.Repo{{Name: "a"}})
	for _, size := range [][2]int{{1, 1}, {10, 3}, {60, 8}, {200, 50}} {
		m.width, m.height = size[0], size[1]
		if m.View() == "" {
			t.Errorf("empty view at %v", size)
		}
	}
}

func TestStalePreviewIsDropped(t *testing.T) {
	m := newTestModel(t)
	m.setRepos([]repo.Repo{{Name: "a"}})
	next, _ := m.Update(previewMsg{gen: m.gen - 1, name: "a", status: "old"})
	if _, ok := next.(Model).previews["a"]; ok {
		t.Fatal("stale preview cached")
	}
}

func TestFilterBackspaceDeletesWholeRune(t *testing.T) {
	m := newTestModel(t)
	m.filtering, m.filter = true, "ab\u00e9"
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyBackspace})
	if got := next.(Model).filter; got != "ab" {
		t.Fatalf("want %q, got %q", "ab", got)
	}
}

func TestCtrlCQuitsWhileFiltering(t *testing.T) {
	m := newTestModel(t)
	m.filtering = true
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	if cmd == nil {
		t.Fatal("expected quit command")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatal("expected tea.QuitMsg")
	}
}

func TestTabCreatedReloadsHerdr(t *testing.T) {
	m := newTestModel(t)
	if _, cmd := m.Update(tabCreatedMsg{name: "a"}); cmd == nil {
		t.Fatal("expected a herdr reload command")
	}
}

func TestSingleRepoOperationIsExclusive(t *testing.T) {
	m := newTestModel(t)
	m.setRepos([]repo.Repo{{Name: "a"}})
	if m.gitOp("pull", "pull") == nil {
		t.Fatal("first operation should start")
	}
	if m.gitOp("push", "push") != nil {
		t.Fatal("second operation must wait for the first")
	}
	next, cmd := m.Update(opDoneMsg{op: "pull", name: "a"})
	if next.(Model).busy != "" || cmd == nil {
		t.Fatal("operation end should clear busy and rescan")
	}
}

func TestCommitDialog(t *testing.T) {
	m := newTestModel(t)
	m.setRepos([]repo.Repo{{Name: "a", Changes: 2}, {Name: "clean"}})

	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("c")})
	m = next.(Model)
	if !m.committing {
		t.Fatal("c on a dirty repo should open the dialog")
	}
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd != nil || !next.(Model).committing {
		t.Fatal("empty message must not commit")
	}
	for _, r := range "Fixed it" {
		next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		m = next.(Model)
	}
	if m.commitMsg != "Fixed it" {
		t.Fatalf("typed message: %q", m.commitMsg)
	}
	if m.width, m.height = 100, 30; m.View() == "" {
		t.Fatal("dialog view is empty")
	}
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = next.(Model)
	if m.committing {
		t.Fatal("esc should close the dialog")
	}

	m.cursor = 1 // clean repo
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("c")})
	if next.(Model).committing {
		t.Fatal("c on a clean repo must not open the dialog")
	}
}
