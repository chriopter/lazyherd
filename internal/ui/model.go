// Package ui is the Bubble Tea front end of lazyherd: a list of repositories
// that drives a lazygit pane next to it.
package ui

import (
	"fmt"
	"hash/fnv"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/chriopter/lazyherd/internal/herdr"
	"github.com/chriopter/lazyherd/internal/repo"
)

// Model is the whole application state.
type Model struct {
	root         string
	repos        []repo.Repo
	visible      []int         // indices into repos that pass the filters
	cursor       int           // index into visible
	commits      []repo.Commit // newest commits across all repos, for the Changes tab
	changes      []repo.Commit // rows of the Changes tab: uncommitted work, then commits, filtered
	changeCursor int           // index into changes
	herdr        herdr.State
	pins         *pins

	ownPane       string         // HERDR_PANE_ID, the pane lazyherd runs in
	companionPane string         // Herdr pane running the follower, "" until it is up
	companion     io.WriteCloser // connection to the follower
	starting      bool           // companion pane is on its way
	selSeq        int            // bumped on every selection change, for debouncing
	shown         string         // what the companion currently shows: a repo, or a repo and change

	theme   theme
	version string
	spin    int   // spinner frame while something runs
	now     int64 // unix time of the last scan, for the age column

	gen           int    // bumped on every rescan; stale results are dropped
	activity      string // what runs right now: "scanning", "fetching", "syncing …", or "" when idle
	width, height int
	filtering     bool   // typing into the name filter
	filter        string // current name filter
	workspaceOnly bool   // only repos of the current Herdr workspace (w toggles)
	timeline      bool   // the Changes tab is open instead of the Repos tab
	timelineSeq   int    // bumped on every timeline load; older results are dropped
	status        string // transient note in the title bar
}

// New creates the model for a directory of repositories, styled after the
// user's lazygit configuration.
func New(root, version string) Model {
	m := newModel(root, pinsPath())
	m.version = version
	m.theme = newTheme(loadConfig(lazygitConfigPath()))
	return m
}

func newModel(root, pinPath string) Model {
	store, err := loadPins(pinPath)
	m := Model{
		root:     root,
		pins:     store,
		ownPane:  os.Getenv("HERDR_PANE_ID"),
		theme:    newTheme(defaultConfig()),
		version:  "dev",
		gen:      1,
		activity: "scanning", // Init scans …
		starting: true,       // … and opens the companion pane
	}
	if err != nil {
		m.status = "pins: " + err.Error()
	}
	return m
}

// Init starts the first scan, the companion pane and the timers.
func (m Model) Init() tea.Cmd {
	return tea.Batch(scanCmd(m.gen, m.root), herdrCmd(m.gen, m.root), refreshTick(), fetchTick(), spinTick(), companionCmd(m.root, m.ownPane))
}

// restartCompanion opens the lazygit pane again after it was lost.
func (m *Model) restartCompanion() tea.Cmd {
	if m.starting {
		return nil
	}
	m.starting = true
	return companionCmd(m.root, m.ownPane)
}

// scanCmds starts a scan unless something else is running.
func (m *Model) scanCmds() tea.Cmd {
	if !m.idle() {
		return nil
	}
	m.activity = "scanning"
	return tea.Batch(scanCmd(m.gen, m.root), herdrCmd(m.gen, m.root))
}

// rescan starts a new scan generation; a running scan's result is dropped.
func (m *Model) rescan() tea.Cmd {
	m.gen++
	m.activity = ""
	return m.scanCmds()
}

// current is the selected repo: the selected row of the Repos tab, or the
// repo of the selected change in the Changes tab.
func (m Model) current() *repo.Repo {
	if m.timeline {
		c := m.currentChange()
		if c == nil {
			return nil
		}
		for i := range m.repos {
			if m.repos[i].Name == c.Repo {
				return &m.repos[i]
			}
		}
		return nil
	}
	if m.cursor >= len(m.visible) {
		return nil
	}
	return &m.repos[m.visible[m.cursor]]
}

// currentChange is the selected row of the Changes tab.
func (m Model) currentChange() *repo.Commit {
	if m.changeCursor >= len(m.changes) {
		return nil
	}
	return &m.changes[m.changeCursor]
}

// rowCount is the number of rows in the open tab.
func (m Model) rowCount() int {
	if m.timeline {
		return len(m.changes)
	}
	return len(m.visible)
}

// cursorPos is the selected row of the open tab.
func (m Model) cursorPos() int {
	if m.timeline {
		return m.changeCursor
	}
	return m.cursor
}

// activeCursor is the cursor of the open tab.
func (m *Model) activeCursor() *int {
	if m.timeline {
		return &m.changeCursor
	}
	return &m.cursor
}

// setTab opens the Repos tab or the Changes tab. The selected repo stays:
// the Changes tab opens at its newest change, the Repos tab at the repo of
// the selected change.
func (m *Model) setTab(changes bool) tea.Cmd {
	if m.timeline == changes {
		return nil
	}
	keep := ""
	if r := m.current(); r != nil {
		keep = r.Name
	}
	m.timeline = changes
	m.applyFilter(keep)
	m.applyChangeFilter("", keep)
	if changes {
		return tea.Batch(m.selected(), m.loadTimeline())
	}
	return m.selected()
}

// loadTimeline reads the commits of every repo for the Changes tab.
func (m *Model) loadTimeline() tea.Cmd {
	m.timelineSeq++
	return timelineCmd(m.timelineSeq, m.root, m.repos)
}

// setCommits replaces the commits and keeps the selected change.
func (m *Model) setCommits(commits []repo.Commit) {
	keep, keepHash := m.selection()
	m.commits = commits
	m.applyFilter(keep)
	m.applyChangeFilter(keepHash, keep)
}

func (m Model) currentDir() string { return filepath.Join(m.root, m.current().Name) }

// inWorkspace reports whether a repo belongs to the current Herdr workspace:
// Herdr has a pane in it, or the user pinned it.
func (m Model) inWorkspace(name string) bool {
	return m.herdr.Pane(name, m.herdr.Workspace) != nil || m.pinned(name)
}

// pinned reports whether the user pinned a repo to the current workspace.
func (m Model) pinned(name string) bool {
	return m.pins.has(m.herdr.WorkspaceLabel(), name)
}

// setRepos replaces the repository list and keeps the selection by name.
func (m *Model) setRepos(repos []repo.Repo) {
	keep, keepHash := m.selection()
	m.repos = repos
	m.applyFilter(keep)
	m.applyChangeFilter(keepHash, keep)
}

// passes reports whether a repo passes the name and workspace filters.
func (m Model) passes(name string) bool {
	if f := strings.ToLower(m.filter); f != "" && !strings.Contains(strings.ToLower(name), f) {
		return false
	}
	return !m.workspaceOnly || m.inWorkspace(name)
}

// applyFilter recomputes the visible rows and selects the repo named keep.
// With a Herdr workspace the workspace's repos are grouped first.
func (m *Model) applyFilter(keep string) {
	m.visible = m.visible[:0]
	for i, r := range m.repos {
		if m.passes(r.Name) {
			m.visible = append(m.visible, i)
		}
	}
	if !m.workspaceOnly {
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

// applyChangeFilter recomputes the rows of the Changes tab, uncommitted
// work of the listed repos first, then their commits, and selects the change
// keep of keepRepo, or else keepRepo's newest change.
func (m *Model) applyChangeFilter(keep, keepRepo string) {
	m.changes = m.changes[:0]
	for _, i := range m.visible {
		if r := m.repos[i]; r.Err == nil && r.Dirty() {
			m.changes = append(m.changes, repo.Commit{Repo: r.Name, Hash: repo.WorkTree, Time: m.now})
		}
	}
	for _, c := range m.commits {
		if m.passes(c.Repo) {
			m.changes = append(m.changes, c)
		}
	}
	m.changeCursor = 0
	byRepo := -1
	for i, c := range m.changes {
		if c.Hash == keep && c.Repo == keepRepo {
			m.changeCursor = i
			return
		} else if byRepo < 0 && c.Repo == keepRepo {
			byRepo = i
		}
	}
	if byRepo >= 0 {
		m.changeCursor = byRepo
	}
}

// selection names the selected repo and, in the Changes tab, the selected
// change, for keeping them across a refresh.
func (m Model) selection() (name, hash string) {
	if r := m.current(); r != nil {
		name = r.Name
	}
	if c := m.currentChange(); c != nil {
		hash = c.Hash
	} else if m.cursor < len(m.visible) {
		// No change selected yet, e.g. the commits are still loading: keep
		// the repo the Repos tab had selected.
		name = m.repos[m.visible[m.cursor]].Name
	}
	return name, hash
}

func (m *Model) refilter() {
	keep, keepHash := m.selection()
	m.applyFilter(keep)
	m.applyChangeFilter(keepHash, keep)
}

// selected schedules the companion update for the current selection.
func (m *Model) selected() tea.Cmd {
	if m.companion == nil {
		return nil
	}
	m.selSeq++
	return selectionTick(m.selSeq)
}

// showCurrent tells the companion pane what to show: lazygit for the
// selected repo, or in the Changes tab the selected change. A dead
// connection drops the companion, so that Enter opens a new one.
func (m *Model) showCurrent() {
	r := m.current()
	if m.companion == nil || r == nil {
		return
	}
	key, send := r.Name, func() error { return herdr.SendSelection(m.companion, m.currentDir()) }
	if c := m.currentChange(); m.timeline && c != nil {
		rev := c.Hash
		if rev == repo.WorkTree {
			// Uncommitted work changes under the pane: show it again when
			// the files change.
			rev += ":" + worktreeVersion(*r)
		}
		key = r.Name + "\x1f" + rev
		send = func() error { return herdr.SendChange(m.companion, m.currentDir(), rev) }
	}
	if key == m.shown {
		return
	}
	if err := send(); err != nil {
		m.status = "lazygit pane is gone, Enter opens a new one"
		m.companion, m.companionPane, m.shown = nil, "", ""
		return
	}
	m.shown = key
}

// worktreeVersion identifies the state of a repo's uncommitted changes.
func worktreeVersion(r repo.Repo) string {
	h := fnv.New32a()
	fmt.Fprint(h, r.Head)
	for _, c := range r.Changes {
		fmt.Fprint(h, c.Code(), c.Path, "\x00")
	}
	return fmt.Sprintf("%08x", h.Sum32())
}

// idle reports whether nothing is running that a background job could disturb.
func (m Model) idle() bool { return m.activity == "" }

// canSync reports whether a sync may start: not while another sync or a
// fetch runs. A scan in flight is harmless, its result is simply refreshed.
func (m Model) canSync() bool { return m.activity == "" || m.activity == "scanning" }

// start records a running job and returns its command with the spinner.
func (m *Model) start(activity string, cmd tea.Cmd) tea.Cmd {
	m.activity = activity
	return tea.Batch(cmd, spinTick())
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
		m.activity = ""
		m.now = time.Now().Unix()
		if msg.err != nil {
			m.status = "scan failed: " + msg.err.Error()
		}
		m.setRepos(msg.repos)
		if m.timeline {
			return m, tea.Batch(m.selected(), m.loadTimeline())
		}
		return m, m.selected()

	case timelineMsg:
		if msg.seq != m.timelineSeq {
			return m, nil
		}
		m.setCommits(msg.commits)
		return m, m.selected()

	case herdrMsg:
		if msg.gen != m.gen {
			return m, nil
		}
		if msg.err != nil {
			// Keep what we knew; the next refresh asks again.
			m.status = "herdr: " + msg.err.Error()
			return m, nil
		}
		m.herdr = msg.state
		m.refilter()
		return m, m.selected()

	case companionMsg:
		m.starting = false
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
		return m, tea.Batch(m.scanCmds(), refreshTick())

	case autoFetchMsg:
		if !m.idle() {
			return m, fetchTick()
		}
		return m, tea.Batch(m.start("fetching", fetchCmd(m.root, m.repos)), fetchTick())

	case fetchDoneMsg:
		m.activity = ""
		if len(msg.failed) > 0 {
			m.status = "fetch failed for " + strings.Join(msg.failed, ", ")
		}
		return m, m.scanCmds()

	case syncDoneMsg:
		m.activity = ""
		m.status = syncSummary(msg)
		return m, m.rescan()

	case tabCreatedMsg:
		m.status = "opened " + msg.name
		return m, herdrCmd(m.gen, m.root)

	case statusMsg:
		m.status = string(msg)

	case tea.MouseMsg:
		if msg.Action == tea.MouseActionPress && msg.Button == tea.MouseButtonLeft {
			if changes, ok := m.tabAt(msg.X, msg.Y); ok {
				return m, m.setTab(changes)
			}
			if i, ok := m.repoRowAt(msg.Y); ok {
				*m.activeCursor() = i
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
		cur := m.activeCursor()
		*cur = min(*cur+1, max(0, m.rowCount()-1))
	case "up", "k":
		cur := m.activeCursor()
		*cur = max(*cur-1, 0)
	case "g", "home":
		*m.activeCursor() = 0
	case "G", "end":
		*m.activeCursor() = max(0, m.rowCount()-1)

	case "tab":
		return m, m.setTab(!m.timeline)
	case "[":
		return m, m.setTab(false)
	case "]":
		return m, m.setTab(true)

	case "/":
		m.filtering = true

	case "p":
		if r := m.current(); r != nil && m.canSync() {
			return m, m.start("syncing "+r.Name, syncCmd(m.root, []repo.Repo{*r}))
		}
	case "P":
		if m.canSync() && len(m.visible) > 0 {
			return m, m.start(fmt.Sprintf("syncing %d repos", len(m.visible)), syncCmd(m.root, m.visibleRepos()))
		}

	case "w":
		m.workspaceOnly = !m.workspaceOnly
		m.refilter()
	case " ":
		if r := m.current(); r != nil {
			if err := m.pins.toggle(m.herdr.WorkspaceLabel(), r.Name); err != nil {
				m.status = "pins: " + err.Error()
			}
			m.refilter()
		}

	case "enter", "l", "right":
		if m.current() == nil {
			break
		}
		if m.companionPane == "" {
			return m, m.restartCompanion()
		}
		m.showCurrent()
		pane := m.ownPane
		return m, func() tea.Msg { _ = herdr.FocusRight(pane); return nil }

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
	if r == nil {
		return nil
	}
	if p := m.herdr.Pane(r.Name, m.herdr.Workspace); p != nil {
		return focusTabCmd(p.TabID, r.Name)
	}
	return createTabCmd(m.herdr.Workspace, m.currentDir(), r.Name)
}
