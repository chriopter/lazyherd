package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/chriopter/lazyherd/internal/repo"
)

func TestChangesTabFollowsRepoOfSelectedChange(t *testing.T) {
	m := newTestModel(t)
	m.width, m.height = 80, 20
	m.setRepos([]repo.Repo{{Name: "a"}, {Name: "b"}})
	m, _ = press(t, m, "j") // b
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = next.(Model)
	if !m.timeline || cmd == nil {
		t.Fatal("tab should open the Changes tab and load the commits")
	}
	next, _ = m.Update(timelineMsg{seq: m.timelineSeq, commits: []repo.Commit{
		{Repo: "a", Hash: "3", Subject: "newest"},
		{Repo: "b", Hash: "2", Subject: "fix b", Author: "Ada Lovelace"},
		{Repo: "a", Hash: "1", Subject: "old"},
	}})
	m = next.(Model)
	if c := m.currentChange(); c == nil || c.Hash != "2" {
		t.Fatalf("Changes tab should open at b's newest commit, got %+v", c)
	}
	if v := m.View(); !strings.Contains(v, "Repos - Changes") || !strings.Contains(v, "fix b") || !strings.Contains(v, "Ada") {
		t.Fatalf("Changes tab not rendered:\n%s", v)
	}

	m, _ = press(t, m, "j")
	if m.current().Name != "a" {
		t.Fatalf("selection should follow the commit's repo, got %q", m.current().Name)
	}
	next, _ = m.Update(timelineMsg{seq: m.timelineSeq - 1, commits: nil})
	if len(next.(Model).commits) != 3 {
		t.Fatal("stale commits replaced the current ones")
	}

	m.filter = "b"
	m.refilter()
	if len(m.changes) != 1 || m.currentChange().Repo != "b" {
		t.Fatalf("filter should narrow the Changes tab: %+v", m.changes)
	}
	m.filter = ""
	m.refilter()

	m, _ = press(t, m, "[")
	if m.timeline || m.current().Name != "b" {
		t.Fatalf("back in the Repos tab the change's repo stays selected, got %q", m.current().Name)
	}
	m, _ = press(t, m, "]")
	if !m.timeline {
		t.Fatal("] should open the Changes tab")
	}
}

func TestChangesTabListsUncommittedWorkFirst(t *testing.T) {
	m := newTestModel(t)
	m.width, m.height = 80, 20
	m.timeline = true
	m.setRepos([]repo.Repo{
		{Name: "dirty", Status: repo.Status{Changes: []repo.Change{
			{Path: "a", Staged: 'M', Unstaged: '.'},
			{Path: "b", Staged: '.', Unstaged: 'M'},
			{Path: "c", Untracked: true},
		}}},
		{Name: "clean"},
	})
	m.setCommits([]repo.Commit{{Repo: "clean", Hash: "1", Subject: "done"}})
	if len(m.changes) != 2 || m.changes[0].Hash != repo.WorkTree || m.changes[0].Repo != "dirty" {
		t.Fatalf("uncommitted work should come first: %+v", m.changes)
	}
	if v := m.View(); !strings.Contains(v, "1 staged · 1 unstaged · 1 untracked") {
		t.Fatalf("worktree summary missing:\n%s", v)
	}
}

func TestCompanionShowsLazygitOrChange(t *testing.T) {
	m := newTestModel(t)
	buf := &selectionBuffer{}
	m.companion = buf
	m.setRepos([]repo.Repo{{Name: "a", Status: repo.Status{Changes: []repo.Change{{Path: "x", Untracked: true}}}}})
	m.showCurrent()
	m.timeline = true
	m.setCommits([]repo.Commit{{Repo: "a", Hash: "abc"}})
	m.showCurrent() // uncommitted work comes first
	m, _ = press(t, m, "j")
	m.showCurrent()
	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) != 3 || strings.Contains(lines[0], "\x1f") ||
		!strings.HasPrefix(lines[1], repo.WorkTree+":") || !strings.HasPrefix(lines[2], "abc\x1f") {
		t.Fatalf("companion selections: %q", lines)
	}
}

func TestTabsClickAndNarrowTitle(t *testing.T) {
	m := newTestModel(t)
	m.width, m.height = 60, 20
	m.setRepos([]repo.Repo{{Name: "a"}})
	next, _ := m.Update(tea.MouseMsg{X: tabsLeft + 9, Y: statusPanelH, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft})
	if m = next.(Model); !m.timeline {
		t.Fatal("click on Changes should open it")
	}
	next, _ = m.Update(tea.MouseMsg{X: tabsLeft + 1, Y: statusPanelH, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft})
	if m = next.(Model); m.timeline {
		t.Fatal("click on Repos should open it")
	}
	m.width = 24
	if title, _ := m.tabsTitle(lipgloss.NewStyle()); title != "Repos" {
		t.Fatalf("narrow title %q", title)
	}
}

func TestChangesViewFitsAllSizes(t *testing.T) {
	m := newTestModel(t)
	m.timeline = true
	m.filter = "a"
	m.setRepos([]repo.Repo{{Name: "a-very-long-repository-name", Status: repo.Status{Changes: []repo.Change{{Path: "x", Untracked: true}}}}})
	m.setCommits([]repo.Commit{{Repo: "a-very-long-repository-name", Hash: "1", Author: "Someone With A Long Name", Subject: strings.Repeat("long subject ", 20)}})
	for _, w := range []int{24, 30, 40, 52, 80, 200} {
		m.width, m.height = w, 12
		for _, line := range strings.Split(m.View(), "\n") {
			if got := lipgloss.Width(line); got > w {
				t.Fatalf("width %d: line is %d wide: %q", w, got, line)
			}
		}
	}
}
