package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/chriopter/lazyherd/internal/herdr"
	"github.com/chriopter/lazyherd/internal/repo"
)

// newTestModel builds a model outside of any Herdr pane or workspace.
func newTestModel(t *testing.T) Model {
	t.Helper()
	t.Setenv("HERDR_WORKSPACE_ID", "")
	t.Setenv("HERDR_PANE_ID", "")
	return NewWithPins(t.TempDir(), filepath.Join(t.TempDir(), "pins.json"))
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

func key(s string) tea.KeyMsg {
	switch s {
	case "enter":
		return tea.KeyMsg{Type: tea.KeyEnter}
	case "esc":
		return tea.KeyMsg{Type: tea.KeyEsc}
	case "space":
		return tea.KeyMsg{Type: tea.KeySpace, Runes: []rune(" ")}
	}
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
}

func press(t *testing.T, m Model, keys ...string) (Model, tea.Cmd) {
	t.Helper()
	var cmd tea.Cmd
	for _, k := range keys {
		var next tea.Model
		next, cmd = m.Update(key(k))
		m = next.(Model)
	}
	return m, cmd
}

func TestInitialScanIsAccepted(t *testing.T) {
	t.Setenv("PATH", t.TempDir()) // no herdr
	m := newTestModel(t)
	m = run(t, m, m.scanCmds()) // Init also starts the timers, which a test must not wait for
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

func TestFilterIsCaseInsensitiveAndUTF8Safe(t *testing.T) {
	m := newTestModel(t)
	m.setRepos([]repo.Repo{{Name: "Alpha"}, {Name: "beta"}, {Name: "alphabet"}})
	m, _ = press(t, m, "/", "A", "L")
	if len(m.visible) != 2 {
		t.Fatalf("want 2 visible, got %d", len(m.visible))
	}
	m.filter = "abé"
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyBackspace})
	if got := next.(Model).filter; got != "ab" {
		t.Fatalf("backspace should delete a whole rune, got %q", got)
	}
	m, _ = press(t, m, "esc")
	if m.filter != "" || len(m.visible) != 3 {
		t.Fatal("esc should clear the filter")
	}
}

func TestViewSurvivesAnySize(t *testing.T) {
	m := newTestModel(t)
	m.setRepos([]repo.Repo{{Name: "a-very-long-repository-name"}})
	for _, size := range [][2]int{{1, 1}, {10, 3}, {24, 6}, {40, 8}, {60, 20}, {200, 50}} {
		m.width, m.height = size[0], size[1]
		if m.View() == "" {
			t.Errorf("empty view at %v", size)
		}
	}
}

func TestMouseSelectsRow(t *testing.T) {
	m := newTestModel(t)
	m.width, m.height = 60, 20
	m.setRepos([]repo.Repo{{Name: "a"}, {Name: "b"}})
	next, _ := m.Update(tea.MouseMsg{X: 2, Y: repoRowsTop + 1, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft})
	if got := next.(Model).current().Name; got != "b" {
		t.Fatalf("click should select b, got %q", got)
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

func TestSyncKeys(t *testing.T) {
	m := newTestModel(t)
	m.setRepos([]repo.Repo{{Name: "a"}, {Name: "b"}})
	m, cmd := press(t, m, "p")
	if cmd == nil || m.busy != "syncing a" {
		t.Fatalf("p should sync the selected repo, busy=%q", m.busy)
	}
	if _, cmd := press(t, m, "P"); cmd != nil {
		t.Fatal("a second sync must wait for the first")
	}
	next, cmd := m.Update(syncDoneMsg{{Name: "a", Pushed: true}})
	m = next.(Model)
	if m.busy != "" || cmd == nil || !strings.Contains(m.status, "1 pushed") {
		t.Fatalf("sync end should clear busy, report and rescan: busy=%q status=%q", m.busy, m.status)
	}
	m.filter = "b"
	m.refilter()
	m, _ = press(t, m, "P")
	if m.busy != "syncing 1 repos" {
		t.Fatalf("P should sync the listed repos only, busy=%q", m.busy)
	}
}

func TestRefreshKeepsSelectionAndSkipsWhenBusy(t *testing.T) {
	m := newTestModel(t)
	m.setRepos([]repo.Repo{{Name: "a"}, {Name: "b"}})
	m.cursor = 1
	next, _ := m.Update(scanMsg{gen: m.gen, repos: []repo.Repo{{Name: "b", Status: repo.Status{Changes: []repo.Change{{Path: "x"}}}}, {Name: "a"}}})
	m = next.(Model)
	if m.current().Name != "b" {
		t.Fatalf("selection lost on background rescan: %v", m.current())
	}
	m.busy = "syncing b"
	if _, cmd := m.Update(refreshMsg(time.Now())); cmd == nil {
		t.Fatal("refresh should at least re-arm its timer")
	}
}

// herdrStateWith loads a Herdr state through a fake herdr binary that reports
// one pane per named repo in the given workspace.
func herdrStateWith(t *testing.T, root, workspace string, repos ...string) herdr.State {
	t.Helper()
	panes := ""
	for i, r := range repos {
		if i > 0 {
			panes += ","
		}
		panes += `{"pane_id":"p` + r + `","tab_id":"t` + r + `","workspace_id":"` + workspace + `","cwd":"` + root + "/" + r + `"}`
	}
	bin := t.TempDir()
	script := "#!/bin/sh\ncase \"$1 $2\" in\n  \"pane list\") printf '%s' '{\"result\":{\"panes\":[" + panes + "]}}' ;;\n  \"workspace list\") printf '%s' '{\"result\":{\"workspaces\":[]}}' ;;\n  *) exit 1 ;;\nesac\n"
	if err := os.WriteFile(filepath.Join(bin, "herdr"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	return herdr.Load(root)
}

func TestWorkspaceGroupingToggleAndPins(t *testing.T) {
	t.Setenv("HERDR_WORKSPACE_ID", "w1")
	t.Setenv("HERDR_PANE_ID", "")
	root := t.TempDir()
	m := NewWithPins(root, filepath.Join(t.TempDir(), "pins.json"))
	m.setRepos([]repo.Repo{{Name: "a", Status: repo.Status{Changes: []repo.Change{{Path: "x"}}}}, {Name: "b"}, {Name: "c"}})
	next, _ := m.Update(herdrMsg{gen: m.gen, state: herdrStateWith(t, root, "w1", "c")})
	m = next.(Model)
	if m.workspaceOnly || m.repos[m.visible[0]].Name != "c" {
		t.Fatalf("default view should list all repos with the workspace repo first: %v", m.visible)
	}
	m.width, m.height = 80, 20
	if !strings.Contains(m.View(), "⌂") {
		t.Fatal("workspace repos should carry the ⌂ marker")
	}

	m, _ = press(t, m, "w")
	if !m.workspaceOnly || len(m.visible) != 1 {
		t.Fatalf("w should narrow to the workspace: only=%v visible=%d", m.workspaceOnly, len(m.visible))
	}
	m, _ = press(t, m, "w")
	if m.workspaceOnly || len(m.visible) != 3 {
		t.Fatal("w should switch back to all repos")
	}

	m.cursor = 2 // "b"
	if m.current().Name != "b" {
		t.Fatalf("expected b selected, got %s", m.current().Name)
	}
	m, _ = press(t, m, "space")
	if !m.pinned("b") || !m.inWorkspace(m.repos[m.visible[0]].Name) || !m.inWorkspace(m.repos[m.visible[1]].Name) {
		t.Fatal("space should pin the repo into the workspace group")
	}
	m, _ = press(t, m, "w")
	if len(m.visible) != 2 {
		t.Fatalf("workspace view should show pane repo and pinned repo, got %d", len(m.visible))
	}
	m, _ = press(t, m, "space")
	if m.pinned("b") || len(m.visible) != 1 {
		t.Fatal("space again should unpin and drop the repo from the workspace view")
	}
}

func TestSelectionIsDebouncedToCompanion(t *testing.T) {
	m := newTestModel(t)
	m.setRepos([]repo.Repo{{Name: "a"}, {Name: "b"}})
	if _, cmd := press(t, m, "j"); cmd != nil {
		t.Fatal("without a companion no selection timer is needed")
	}
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	m.companion = w
	m, cmd := press(t, m, "j")
	if cmd == nil {
		t.Fatal("a companion needs the debounce timer")
	}
	next, _ := m.Update(selectionMsg(m.selSeq - 1)) // stale timer
	if next.(Model).shown != "" {
		t.Fatal("a stale selection tick must not send")
	}
	next, _ = m.Update(selectionMsg(m.selSeq))
	m = next.(Model)
	w.Close()
	buf := make([]byte, 256)
	n, _ := r.Read(buf)
	if m.shown != "b" || !strings.HasSuffix(strings.TrimSpace(string(buf[:n])), "/b") {
		t.Fatalf("companion should receive the selected repo path, got %q", buf[:n])
	}
}
