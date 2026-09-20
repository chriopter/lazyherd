package ui

import (
	"net"
	"os"
	"path/filepath"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/chriopter/lazyherd/internal/follow"
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
	fetchDoneMsg   struct{ failed []string }
	syncDoneMsg    []repo.SyncResult
	lazygitDoneMsg struct{ err error }
	tabCreatedMsg  struct{ name string }
	companionMsg   struct {
		pane string
		conn net.Conn
		err  error
	}
	selectionMsg int // debounced selection, carries the sequence number
	statusMsg    string
	refreshMsg   time.Time
	autoFetchMsg time.Time
)

const (
	refreshEvery   = 3 * time.Second
	fetchEvery     = 60 * time.Second
	selectionDelay = 250 * time.Millisecond
	companionRatio = 0.3 // share of the width the list keeps; lazygit gets the rest
)

func refreshTick() tea.Cmd {
	return tea.Tick(refreshEvery, func(t time.Time) tea.Msg { return refreshMsg(t) })
}

func fetchTick() tea.Cmd {
	return tea.Tick(fetchEvery, func(t time.Time) tea.Msg { return autoFetchMsg(t) })
}

func selectionTick(seq int) tea.Cmd {
	return tea.Tick(selectionDelay, func(time.Time) tea.Msg { return selectionMsg(seq) })
}

func scanCmd(gen int, root string) tea.Cmd {
	return func() tea.Msg {
		repos, err := repo.Scan(root)
		return scanMsg{gen: gen, repos: repos, err: err}
	}
}

func herdrCmd(gen int, root string) tea.Cmd {
	return func() tea.Msg { return herdrMsg{gen: gen, state: herdr.Load(root)} }
}

func fetchCmd(root string, repos []repo.Repo) tea.Cmd {
	return func() tea.Msg { return fetchDoneMsg{failed: repo.FetchAll(root, repos)} }
}

func syncCmd(root string, repos []repo.Repo) tea.Cmd {
	return func() tea.Msg { return syncDoneMsg(repo.SyncAll(root, repos)) }
}

func lazygitCmd(dir string) tea.Cmd {
	return tea.ExecProcess(repo.Lazygit(dir), func(err error) tea.Msg { return lazygitDoneMsg{err: err} })
}

// companionCmd splits a pane to the right of ours in Herdr and starts the
// follower there, then connects to it.
func companionCmd(root, ownPane string) tea.Cmd {
	return func() tea.Msg {
		exe, err := os.Executable()
		if err != nil {
			return companionMsg{err: err}
		}
		pane, err := herdr.SplitRight(ownPane, root, companionRatio)
		if err != nil {
			return companionMsg{err: err}
		}
		socket := filepath.Join(socketDir(), "lazyherd-"+ownPane+".sock")
		if err := herdr.Run(pane, exe+" follow "+socket); err != nil {
			_ = herdr.ClosePane(pane)
			return companionMsg{err: err}
		}
		conn, err := follow.Dial(socket, 10*time.Second)
		if err != nil {
			_ = herdr.ClosePane(pane)
			return companionMsg{err: err}
		}
		return companionMsg{pane: pane, conn: conn}
	}
}

func socketDir() string {
	if d := os.Getenv("XDG_RUNTIME_DIR"); d != "" {
		return d
	}
	return os.TempDir()
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
