package ui

import (
	"net"
	"time"

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
		err   error
	}
	fetchDoneMsg  struct{ failed []string }
	syncDoneMsg   []repo.SyncResult
	tabCreatedMsg struct{ name string }
	companionMsg  struct {
		pane string
		conn net.Conn
		err  error
	}
	selectionMsg int // debounced selection, carries the sequence number
	statusMsg    string
	refreshMsg   struct{}
	autoFetchMsg struct{}
	spinMsg      struct{}
)

const (
	refreshEvery   = 3 * time.Second
	fetchEvery     = 60 * time.Second
	selectionDelay = 250 * time.Millisecond
	spinEvery      = 180 * time.Millisecond // lazygit's spinner rate
	companionRatio = 0.3                    // share of the width the list keeps; lazygit gets the rest
)

func refreshTick() tea.Cmd {
	return tick(refreshEvery, refreshMsg{})
}

func fetchTick() tea.Cmd {
	return tick(fetchEvery, autoFetchMsg{})
}

func spinTick() tea.Cmd {
	return tick(spinEvery, spinMsg{})
}

func selectionTick(seq int) tea.Cmd {
	return tick(selectionDelay, selectionMsg(seq))
}

func tick(delay time.Duration, msg tea.Msg) tea.Cmd {
	return tea.Tick(delay, func(time.Time) tea.Msg { return msg })
}

func scanCmd(gen int, root string) tea.Cmd {
	return func() tea.Msg {
		repos, err := repo.Scan(root)
		return scanMsg{gen: gen, repos: repos, err: err}
	}
}

func herdrCmd(gen int, root string) tea.Cmd {
	return func() tea.Msg {
		state, err := herdr.Load(root)
		return herdrMsg{gen: gen, state: state, err: err}
	}
}

func fetchCmd(root string, repos []repo.Repo) tea.Cmd {
	return func() tea.Msg { return fetchDoneMsg{failed: repo.FetchAll(root, repos)} }
}

func syncCmd(root string, repos []repo.Repo) tea.Cmd {
	return func() tea.Msg { return syncDoneMsg(repo.SyncAll(root, repos)) }
}

// companionCmd splits a pane to the right of ours in Herdr and starts the
// follower there, then connects to it.
func companionCmd(root, ownPane string) tea.Cmd {
	return func() tea.Msg {
		pane, conn, err := herdr.StartCompanion(root, ownPane, companionRatio)
		return companionMsg{pane: pane, conn: conn, err: err}
	}
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
