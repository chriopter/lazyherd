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

// newTestModel builds a model outside of any Herdr workspace.
func newTestModel(t *testing.T) Model {
	t.Helper()
	t.Setenv("HERDR_WORKSPACE_ID", "")
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
	next, _ := m.Update(previewMsg{gen: m.gen - 1, name: "a", preview: preview{branch: "old"}})
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
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = next.(Model)
	for _, r := range "why" {
		next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		m = next.(Model)
	}
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)
	if m.commitBody != "why\n" || !m.committing {
		t.Fatalf("enter in the description should add a line, got %q committing=%v", m.commitBody, m.committing)
	}
	if !strings.Contains(m.View(), "DESCRIPTION") {
		t.Fatal("description field should always be shown")
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

func TestFilePaneNavigationAndMouse(t *testing.T) {
	m := newTestModel(t)
	m.width, m.height = 120, 30
	m.setRepos([]repo.Repo{{Name: "a", Changes: 2}, {Name: "b"}})
	p := preview{changes: []repo.Change{{Path: "x/one.go", Unstaged: 'M', Staged: ' '}, {Path: "two.go", Untracked: true}}}
	p.rows = buildTree(p.changes)
	p.files = fileRows(p.rows)
	m.previews["a"] = p

	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("l")})
	m = next.(Model)
	if m.focus != paneFiles || m.currentFile().Path != "two.go" {
		t.Fatalf("l should focus the first file, got focus=%v file=%v", m.focus, m.currentFile())
	}
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	m = next.(Model)
	if m.currentFile().Path != "x/one.go" {
		t.Fatalf("j should select the next file, got %v", m.currentFile())
	}
	if !strings.Contains(m.View(), "DIFF x/one.go") {
		t.Fatal("view should show the diff section for the selected file")
	}
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = next.(Model)
	if m.focus != paneRepos {
		t.Fatal("esc should return to the repo pane")
	}

	// click on the first tree row (two.go) inside the right pane
	next, _ = m.Update(tea.MouseMsg{X: m.layout().leftW + 3, Y: treeRowsTop, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft})
	m = next.(Model)
	if m.focus != paneFiles || m.currentFile().Path != "two.go" {
		t.Fatalf("click should select two.go, got focus=%v file=%v", m.focus, m.currentFile())
	}
	// click on the second repo row
	next, _ = m.Update(tea.MouseMsg{X: 2, Y: repoRowsTop + 1, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft})
	m = next.(Model)
	if m.current().Name != "b" || m.focus != paneRepos {
		t.Fatalf("click should select repo b, got %v", m.current())
	}
}

func TestGeneratedMessageFillsDialog(t *testing.T) {
	m := newTestModel(t)
	m.setRepos([]repo.Repo{{Name: "a", Changes: 1}})
	m.committing, m.generating = true, true
	next, _ := m.Update(generatedMsg{name: "a", subject: "Added thing", body: "Because."})
	m = next.(Model)
	if m.generating || m.commitMsg != "Added thing" || m.commitBody != "Because." {
		t.Fatalf("generated message not applied: %q / %q", m.commitMsg, m.commitBody)
	}
	if m.width, m.height = 100, 30; !strings.Contains(m.View(), "Because.") {
		t.Fatal("dialog should show the description")
	}
	m.committing = false
	next, _ = m.Update(generatedMsg{name: "a", subject: "late"})
	if next.(Model).commitMsg == "late" {
		t.Fatal("a late result must not change a closed dialog")
	}
}

func TestSplitMessage(t *testing.T) {
	sub, body := splitMessage("```\n\"Fixed the thing\"\n\nIt was broken because of X.\n- also Y\n```")
	if sub != "Fixed the thing" || body != "It was broken because of X.\n- also Y" {
		t.Fatalf("got %q / %q", sub, body)
	}
	if sub, body := splitMessage("Only subject"); sub != "Only subject" || body != "" {
		t.Fatalf("got %q / %q", sub, body)
	}
}

func TestReflow(t *testing.T) {
	in := "Swap the README (fork notice,\ndev commands) for a minimal\nHello World.\n\n- first bullet\n  continued\n- second"
	want := "Swap the README (fork notice, dev commands) for a minimal Hello World.\n\n- first bullet continued\n- second"
	if got := reflow(in); got != want {
		t.Fatalf("got %q", got)
	}
}

func TestWorkspaceToggle(t *testing.T) {
	t.Setenv("HERDR_WORKSPACE_ID", "w1")
	m := NewWithPins(t.TempDir(), filepath.Join(t.TempDir(), "pins.json"))
	m.setRepos([]repo.Repo{{Name: "a"}, {Name: "b"}})
	next, _ := m.Update(herdrMsg{gen: m.gen, state: herdr.State{Available: true, Workspace: "w1"}})
	m = next.(Model)
	if m.workspaceOnly {
		t.Fatal("a workspace without repo panes should start with all repos")
	}
	if len(m.visible) != 2 {
		t.Fatalf("want 2 visible, got %d", len(m.visible))
	}
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("w")})
	m = next.(Model)
	if !m.workspaceOnly || len(m.visible) != 0 {
		t.Fatalf("w should switch to workspace mode: only=%v visible=%d", m.workspaceOnly, len(m.visible))
	}
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("w")})
	m = next.(Model)
	if m.workspaceOnly || len(m.visible) != 2 {
		t.Fatal("w should switch back to all repos")
	}
}

func TestWorkspaceReposAreGroupedFirst(t *testing.T) {
	t.Setenv("HERDR_WORKSPACE_ID", "w1")
	root := t.TempDir()
	m := NewWithPins(root, filepath.Join(t.TempDir(), "pins.json"))
	m.setRepos([]repo.Repo{{Name: "a", Changes: 5}, {Name: "b"}, {Name: "c", Changes: 1}})
	st := herdrStateWith(root, "w1", "c")
	next, _ := m.Update(herdrMsg{gen: m.gen, state: st})
	m = next.(Model)
	if m.workspaceOnly {
		t.Fatal("workspace mode should be off when the view is started with a grouped list")
	}
	if got := m.repos[m.visible[0]].Name; got != "c" {
		t.Fatalf("workspace repo should come first, got %q", got)
	}
	if !m.inWorkspace("c") || m.inWorkspace("a") {
		t.Fatal("inWorkspace wrong")
	}
	m.width, m.height = 120, 30
	if !strings.Contains(m.View(), "⌂") {
		t.Fatal("workspace repos should carry the ⌂ marker")
	}
}

func TestRefreshKeepsSelectionAndSkipsWhenBusy(t *testing.T) {
	m := newTestModel(t)
	m.setRepos([]repo.Repo{{Name: "a"}, {Name: "b"}})
	m.cursor = 1
	next, _ := m.Update(scanMsg{gen: m.gen, repos: []repo.Repo{{Name: "b", Changes: 3}, {Name: "a"}}})
	m = next.(Model)
	if m.current().Name != "b" {
		t.Fatalf("selection lost on background rescan: %v", m.current())
	}
	m.busy = "pull b"
	if _, cmd := m.Update(refreshMsg(time.Now())); cmd == nil {
		t.Fatal("refresh should at least re-arm its timer")
	}
}

// herdrStateWith loads a Herdr state through a fake herdr binary that reports
// one pane per named repo in the given workspace.
func herdrStateWith(t_root, workspace string, repos ...string) herdr.State {
	panes := ""
	for i, r := range repos {
		if i > 0 {
			panes += ","
		}
		panes += `{"pane_id":"p` + r + `","tab_id":"t` + r + `","workspace_id":"` + workspace + `","cwd":"` + t_root + "/" + r + `"}`
	}
	bin, _ := os.MkdirTemp("", "fakeherdr")
	script := "#!/bin/sh\ncase \"$1 $2\" in\n  \"pane list\") printf '%s' '{\"result\":{\"panes\":[" + panes + "]}}' ;;\n  \"workspace list\") printf '%s' '{\"result\":{\"workspaces\":[]}}' ;;\n  *) exit 1 ;;\nesac\n"
	os.WriteFile(bin+"/herdr", []byte(script), 0o755)
	os.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	return herdr.Load(t_root)
}

func TestSpacePinsRepoToWorkspace(t *testing.T) {
	t.Setenv("HERDR_WORKSPACE_ID", "w1")
	root := t.TempDir()
	m := NewWithPins(root, filepath.Join(t.TempDir(), "pins.json"))
	m.setRepos([]repo.Repo{{Name: "a"}, {Name: "b"}, {Name: "c"}})
	next, _ := m.Update(herdrMsg{gen: m.gen, state: herdrStateWith(root, "w1", "c")})
	m = next.(Model)
	m.selectRepo(2) // grouping puts c first, then a, b
	if m.current().Name != "b" {
		t.Fatalf("expected b selected, got %s", m.current().Name)
	}
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeySpace, Runes: []rune(" ")})
	m = next.(Model)
	if !m.pinned("b") || !m.inWorkspace("b") {
		t.Fatal("space should pin the repo")
	}
	if !m.inWorkspace(m.repos[m.visible[0]].Name) || !m.inWorkspace(m.repos[m.visible[1]].Name) {
		t.Fatalf("pinned repo should join the workspace group, order: %v", m.visible)
	}
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("w")})
	m = next.(Model)
	if len(m.visible) != 2 {
		t.Fatalf("workspace view should show pane repo and pinned repo, got %d", len(m.visible))
	}
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeySpace, Runes: []rune(" ")})
	m = next.(Model)
	if m.pinned("b") || len(m.visible) != 1 {
		t.Fatal("space again should unpin and drop the repo from the workspace view")
	}
}
