package ui

import (
	"path/filepath"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/chriopter/lazyherd/internal/herdr"
	"github.com/chriopter/lazyherd/internal/repo"
)

// Messages carry a generation so results of superseded scans are dropped.
type (
	scanMsg struct {
		gen   int
		repos []repo.Repo
		err   error
	}
	herdrMsg struct {
		gen   int
		state herdr.State
	}
	previewMsg struct {
		gen     int
		name    string
		preview preview
	}
	diffMsg struct {
		gen        int
		name, path string
		text       string
	}
	fetchDoneMsg   struct{ failed []string }
	lazygitDoneMsg struct{ err error }
	tabCreatedMsg  struct{ name string }
	opDoneMsg      struct {
		op, name string
		err      error
	}
	statusMsg string
)

const previewCommits = 14

func scanCmd(gen int, root string) tea.Cmd {
	return func() tea.Msg {
		repos, err := repo.Scan(root)
		return scanMsg{gen: gen, repos: repos, err: err}
	}
}

func herdrCmd(gen int, root string) tea.Cmd {
	return func() tea.Msg { return herdrMsg{gen: gen, state: herdr.Load(root)} }
}

func previewCmd(gen int, root, name string) tea.Cmd {
	return func() tea.Msg {
		dir := filepath.Join(root, name)
		p := preview{}
		var err error
		p.branch, p.changes, err = repo.Status(dir)
		if err != nil {
			p.err = err
		}
		p.log = repo.Log(dir, previewCommits)
		p.rows = buildTree(p.changes)
		p.files = fileRows(p.rows)
		return previewMsg{gen: gen, name: name, preview: p}
	}
}

func diffCmd(gen int, root, name string, c repo.Change) tea.Cmd {
	return func() tea.Msg {
		return diffMsg{gen: gen, name: name, path: c.Path, text: repo.Diff(filepath.Join(root, name), c)}
	}
}

func fetchCmd(root string, repos []repo.Repo) tea.Cmd {
	return func() tea.Msg { return fetchDoneMsg{failed: repo.FetchAll(root, repos)} }
}

// gitOpCmd runs a named git operation in one repo, for p, P and f.
func gitOpCmd(op, root, name string, args ...string) tea.Cmd {
	return func() tea.Msg {
		return opDoneMsg{op: op, name: name, err: repo.Run(filepath.Join(root, name), args...)}
	}
}

func commitCmd(root, name, subject, body string) tea.Cmd {
	return func() tea.Msg {
		return opDoneMsg{op: "commit", name: name, err: repo.Commit(filepath.Join(root, name), subject, body)}
	}
}

func lazygitCmd(dir string) tea.Cmd {
	return tea.ExecProcess(repo.Lazygit(dir), func(err error) tea.Msg { return lazygitDoneMsg{err: err} })
}

func focusTabCmd(tabID, name string) tea.Cmd {
	return func() tea.Msg {
		if err := herdr.FocusTab(tabID); err != nil {
			return statusMsg("herdr: " + err.Error())
		}
		return statusMsg("→ " + name)
	}
}

func createTabCmd(workspace, dir, name string) tea.Cmd {
	return func() tea.Msg {
		if err := herdr.CreateTab(workspace, dir, name); err != nil {
			return statusMsg("herdr: " + err.Error())
		}
		return tabCreatedMsg{name: name}
	}
}
