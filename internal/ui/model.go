// Package ui is the Bubble Tea front end of lazyherd: a list of repositories
// that drives a lazygit pane next to it.
package ui

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/chriopter/lazyherd/internal/follow"
	"github.com/chriopter/lazyherd/internal/herdr"
	"github.com/chriopter/lazyherd/internal/lazygit"
	"github.com/chriopter/lazyherd/internal/pins"
	"github.com/chriopter/lazyherd/internal/repo"
)

// Model is the whole application state.
type Model struct {
	root    string
	repos   []repo.Repo
	visible []int // indices into repos that pass the filters
	cursor  int   // index into visible
	herdr   herdr.State
	pins    *pins.Store

	ownPane       string         // HERDR_PANE_ID when running inside Herdr
	companionPane string         // Herdr pane running the follower, "" without one
	companion     io.WriteCloser // connection to the follower
	selSeq        int            // bumped on every selection change, for debouncing
	shown         string         // repo the companion currently shows

	theme   theme
	version string
	spin    int // spinner frame while something runs

	gen           int // bumped on every rescan; stale results are dropped
	width, height int
	loading       bool
	fetching      bool   // background fetch in progress
	busy          string // running sync, e.g. "sync api"
	filtering     bool   // typing into the name filter
	filter        string // current name filter
	workspaceOnly bool   // only repos of the current Herdr workspace (w toggles)
	status        string // transient note in the title bar
}

// New creates the model for a directory of repositories, styled after the
// user's lazygit configuration.
func New(root, version string) Model {
	m := NewWithPins(root, pins.Path())
	m.version = version
	m.theme = newTheme(lazygit.Load(lazygit.Path()))
	return m
}

// NewWithPins builds a model with an explicit pin file and default theme, for tests.
func NewWithPins(root, pinPath string) Model {
	store, _ := pins.Load(pinPath) // an unreadable file just means no pins
	return Model{
		root:    root,
		pins:    store,
		ownPane: os.Getenv("HERDR_PANE_ID"),
		theme:   newTheme(lazygit.Default()),
		version: "dev",
		gen:     1,
		loading: true,
	}
}

// Init starts the first scan, the Herdr companion pane and the timers.
func (m Model) Init() tea.Cmd {
	cmds := []tea.Cmd{m.scanCmds(), refreshTick(), fetchTick(), spinTick()}
	if m.ownPane != "" {
		cmds = append(cmds, companionCmd(m.root, m.ownPane))
	}
	return tea.Batch(cmds...)
}

func (m Model) scanCmds() tea.Cmd {
	return tea.Batch(scanCmd(m.gen, m.root), herdrCmd(m.gen, m.root))
}

// rescan starts a new scan generation.
func (m *Model) rescan() tea.Cmd {
	m.gen++
	m.loading = true
	return m.scanCmds()
}

func (m Model) current() *repo.Repo {
	if m.cursor >= len(m.visible) {
		return nil
	}
	return &m.repos[m.visible[m.cursor]]
}

func (m Model) currentDir() string { return filepath.Join(m.root, m.current().Name) }

// inWorkspace reports whether a repo belongs to the current Herdr workspace:
// Herdr has a pane in it, or the user pinned it.
func (m Model) inWorkspace(name string) bool {
	if m.herdr.Workspace == "" {
		return false
	}
	return m.herdr.Pane(name, m.herdr.Workspace) != nil || m.pinned(name)
}

// pinned reports whether the user pinned a repo to the current workspace.
func (m Model) pinned(name string) bool {
	return m.pins != nil && m.pins.Pinned(m.herdr.WorkspaceLabel(), name)
}

// setRepos replaces the repository list and keeps the selection by name.
func (m *Model) setRepos(repos []repo.Repo) {
	keep := ""
	if r := m.current(); r != nil {
		keep = r.Name
	}
	m.repos = repos
	m.applyFilter(keep)
}

// applyFilter recomputes the visible rows and selects the repo named keep.
// With a Herdr workspace the workspace's repos are grouped first.
func (m *Model) applyFilter(keep string) {
	m.visible = m.visible[:0]
	f := strings.ToLower(m.filter)
	for i, r := range m.repos {
		if f != "" && !strings.Contains(strings.ToLower(r.Name), f) {
			continue
		}
		if m.workspaceOnly && !m.inWorkspace(r.Name) {
			continue
		}
		m.visible = append(m.visible, i)
	}
	if m.herdr.Workspace != "" && !m.workspaceOnly {
		sort.SliceStable(m.visible, func(a, b int) bool {
			return m.inWorkspace(m.repos[m.visible[a]].Name) && !m.inWorkspace(m.repos[m.visible[b]].Name)
		})
	}
	m.cursor = 0
	for i, idx := range m.visible {
		if m.repos[idx].Name == keep {
			m.cursor = i
		}
	}
}

func (m *Model) refilter() {
	keep := ""
	if r := m.current(); r != nil {
		keep = r.Name
	}
	m.applyFilter(keep)
}

// selected schedules the companion update for the current selection.
func (m *Model) selected() tea.Cmd {
	if m.companion == nil {
		return nil
	}
	m.selSeq++
	return selectionTick(m.selSeq)
}

// showCurrent tells the companion pane to open the selected repo.
func (m *Model) showCurrent() {
	r := m.current()
	if m.companion == nil || r == nil || r.Name == m.shown {
		return
	}
	if err := follow.Send(m.companion, m.currentDir()); err != nil {
		m.status = "companion: " + err.Error()
		m.companion = nil
		return
	}
	m.shown = r.Name
}

// idle reports whether nothing is running that a background refresh could disturb.
func (m Model) idle() bool {
	return m.busy == "" && !m.fetching && !m.loading
}

// visibleRepos returns the repos currently listed, for the "all" actions.
func (m Model) visibleRepos() []repo.Repo {
	out := make([]repo.Repo, 0, len(m.visible))
	for _, i := range m.visible {
		out = append(out, m.repos[i])
	}
	return out
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height

	case scanMsg:
		if msg.gen != m.gen {
			return m, nil
		}
		m.loading = false
		if msg.err != nil {
			m.status = "scan failed: " + msg.err.Error()
		}
		m.setRepos(msg.repos)
		return m, m.selected()

	case herdrMsg:
		if msg.gen != m.gen {
			return m, nil
		}
		m.herdr = msg.state
		if m.workspaceOnly && !m.herdr.Available {
			m.workspaceOnly = false
			m.status = "herdr not reachable, showing all repos"
		}
		m.refilter()
		return m, m.selected()

	case companionMsg:
		if msg.err != nil {
			m.status = "no companion pane: " + msg.err.Error()
			return m, nil
		}
		m.companionPane, m.companion = msg.pane, msg.conn
		return m, m.selected()

	case selectionMsg:
		if int(msg) == m.selSeq {
			m.showCurrent()
		}

	case spinMsg:
		if !m.idle() {
			m.spin++
			return m, spinTick()
		}

	case refreshMsg:
		if !m.idle() {
			return m, refreshTick()
		}
		return m, tea.Batch(scanCmd(m.gen, m.root), herdrCmd(m.gen, m.root), refreshTick())

	case autoFetchMsg:
		if !m.idle() {
			return m, fetchTick()
		}
		m.fetching = true
		return m, tea.Batch(fetchCmd(m.root, m.repos), fetchTick(), spinTick())

	case fetchDoneMsg:
		m.fetching = false
		if len(msg.failed) > 0 {
			m.status = "fetch failed for " + strings.Join(msg.failed, ", ")
		}
		return m, scanCmd(m.gen, m.root)

	case syncDoneMsg:
		m.busy = ""
		m.status = syncSummary(msg)
		return m, m.rescan()

	case lazygitDoneMsg:
		if msg.err != nil {
			m.status = "lazygit: " + msg.err.Error()
		}
		return m, m.rescan()

	case tabCreatedMsg:
		m.status = "opened " + msg.name
		return m, herdrCmd(m.gen, m.root)

	case statusMsg:
		m.status = string(msg)

	case tea.MouseMsg:
		if msg.Action == tea.MouseActionPress && msg.Button == tea.MouseButtonLeft {
			if i, ok := m.repoRowAt(msg.Y); ok {
				m.cursor = i
				return m, m.selected()
			}
		}

	case tea.KeyMsg:
		if msg.String() == "ctrl+c" {
			return m, m.quit()
		}
		if m.filtering {
			return m.updateFilter(msg)
		}
		return m.updateKeys(msg)
	}
	return m, nil
}

// quit closes the companion pane before leaving.
func (m Model) quit() tea.Cmd {
	if m.companion != nil {
		m.companion.Close()
	}
	if m.companionPane != "" {
		pane := m.companionPane
		return tea.Sequence(func() tea.Msg { _ = herdr.ClosePane(pane); return nil }, tea.Quit)
	}
	return tea.Quit
}

func (m Model) updateFilter(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.filtering, m.filter = false, ""
	case "enter":
		m.filtering = false
	case "backspace":
		if m.filter != "" {
			_, size := utf8.DecodeLastRuneInString(m.filter)
			m.filter = m.filter[:len(m.filter)-size]
		}
	default:
		if msg.Type == tea.KeyRunes {
			m.filter += string(msg.Runes)
		}
	}
	m.refilter()
	return m, m.selected()
}

func (m Model) updateKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	m.status = ""
	switch msg.String() {
	case "q", "esc":
		if m.filter != "" {
			m.filter = ""
			m.refilter()
			return m, m.selected()
		}
		return m, m.quit()

	case "down", "j":
		m.cursor = min(m.cursor+1, max(0, len(m.visible)-1))
	case "up", "k":
		m.cursor = max(m.cursor-1, 0)
	case "g", "home":
		m.cursor = 0
	case "G", "end":
		m.cursor = max(0, len(m.visible)-1)

	case "/":
		m.filtering = true

	case "p":
		if r := m.current(); r != nil && m.busy == "" {
			m.busy = "syncing " + r.Name
			return m, tea.Batch(syncCmd(m.root, []repo.Repo{*r}), spinTick())
		}
	case "P":
		if m.busy == "" && len(m.visible) > 0 {
			m.busy = fmt.Sprintf("syncing %d repos", len(m.visible))
			return m, tea.Batch(syncCmd(m.root, m.visibleRepos()), spinTick())
		}

	case "w":
		if m.herdr.Workspace != "" && m.herdr.Available {
			m.workspaceOnly = !m.workspaceOnly
			m.refilter()
		}
	case " ":
		if r := m.current(); r != nil && m.herdr.Workspace != "" && m.pins != nil {
			if err := m.pins.Toggle(m.herdr.WorkspaceLabel(), r.Name); err != nil {
				m.status = "pins: " + err.Error()
			}
			m.refilter()
		}

	case "enter", "l", "right":
		if m.current() == nil {
			break
		}
		if m.companionPane != "" {
			m.showCurrent()
			pane := m.ownPane
			return m, func() tea.Msg { _ = herdr.FocusRight(pane); return nil }
		}
		return m, lazygitCmd(m.currentDir())

	case "t":
		return m, m.jumpToHerdr()
	}
	return m, m.selected()
}

// syncSummary condenses sync results into one status line.
func syncSummary(results []repo.SyncResult) string {
	var pulled, pushed int
	var failed []string
	for _, r := range results {
		if r.Pulled {
			pulled++
		}
		if r.Pushed {
			pushed++
		}
		if r.Err != nil {
			failed = append(failed, fmt.Sprintf("%s (%v)", r.Name, r.Err))
		}
	}
	s := fmt.Sprintf("sync: %d pulled, %d pushed", pulled, pushed)
	if len(failed) > 0 {
		s += ", failed: " + strings.Join(failed, ", ")
	}
	return s
}

// jumpToHerdr focuses the Herdr tab holding the selected repo, or opens one.
func (m Model) jumpToHerdr() tea.Cmd {
	r := m.current()
	if r == nil || !m.herdr.Available {
		return nil
	}
	if p := m.herdr.Pane(r.Name, m.herdr.Workspace); p != nil {
		return focusTabCmd(p.TabID, r.Name)
	}
	return createTabCmd(m.herdr.Workspace, m.currentDir(), r.Name)
}
