// Package ui is the Bubble Tea front end of lazyherd.
package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/chriopter/lazyherd/internal/herdr"
	"github.com/chriopter/lazyherd/internal/repo"
)

type preview struct{ status, log string }

// Model is the whole application state.
type Model struct {
	root    string
	repos   []repo.Repo
	visible []int // indices into repos that pass the filters
	cursor  int   // index into visible
	herdr   herdr.State

	gen      int                 // bumped on every rescan; stale results are dropped
	previews map[string]preview  // cached by repo name for the current gen
	pending  map[string]struct{} // previews requested but not yet received

	width, height int
	loading       bool
	fetching      bool   // fetch all in progress
	busy          string // running single-repo operation, e.g. "pull api"
	filtering     bool   // typing into the name filter
	filter        string // current name filter
	committing    bool   // commit dialog open
	commitMsg     string // message typed into the commit dialog
	workspaceOnly bool   // only repos with a pane in the current Herdr workspace
	status        string // transient note in the title bar
}

// New creates the model for a directory of repositories.
func New(root string) Model {
	return Model{
		root:          root,
		gen:           1,
		loading:       true,
		previews:      map[string]preview{},
		pending:       map[string]struct{}{},
		workspaceOnly: os.Getenv("HERDR_WORKSPACE_ID") != "",
	}
}

// Init starts the first scan; New already set the generation it belongs to.
func (m Model) Init() tea.Cmd { return m.scanCmds() }

func (m Model) scanCmds() tea.Cmd {
	return tea.Batch(scanCmd(m.gen, m.root), herdrCmd(m.gen, m.root))
}

// rescan invalidates every cached result and starts a new scan generation.
func (m *Model) rescan() tea.Cmd {
	m.gen++
	m.loading = true
	m.previews = map[string]preview{}
	m.pending = map[string]struct{}{}
	return m.scanCmds()
}

func (m Model) current() *repo.Repo {
	if m.cursor >= len(m.visible) {
		return nil
	}
	return &m.repos[m.visible[m.cursor]]
}

func (m Model) currentDir() string { return filepath.Join(m.root, m.current().Name) }

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
func (m *Model) applyFilter(keep string) {
	m.visible = m.visible[:0]
	f := strings.ToLower(m.filter)
	for i, r := range m.repos {
		if f != "" && !strings.Contains(strings.ToLower(r.Name), f) {
			continue
		}
		if m.workspaceOnly && m.herdr.Pane(r.Name, m.herdr.Workspace) == nil {
			continue
		}
		m.visible = append(m.visible, i)
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

// loadPreview requests the preview of the selected repo unless cached or pending.
func (m *Model) loadPreview() tea.Cmd {
	r := m.current()
	if r == nil {
		return nil
	}
	if _, ok := m.previews[r.Name]; ok {
		return nil
	}
	if _, ok := m.pending[r.Name]; ok {
		return nil
	}
	m.pending[r.Name] = struct{}{}
	return previewCmd(m.gen, m.root, r.Name)
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
		return m, m.loadPreview()

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
		return m, m.loadPreview()

	case previewMsg:
		if msg.gen != m.gen {
			return m, nil
		}
		delete(m.pending, msg.name)
		m.previews[msg.name] = preview{status: msg.status, log: msg.log}

	case fetchDoneMsg:
		m.fetching = false
		cmd := m.rescan()
		if n := len(msg.failed); n > 0 {
			m.status = fmt.Sprintf("fetch failed for %s", strings.Join(msg.failed, ", "))
		}
		return m, cmd

	case lazygitDoneMsg:
		cmd := m.rescan()
		if msg.err != nil {
			m.status = "lazygit: " + msg.err.Error()
		}
		return m, cmd

	case tabCreatedMsg:
		m.status = "opened " + msg.name
		return m, herdrCmd(m.gen, m.root) // the new pane changes the workspace view

	case opDoneMsg:
		m.busy = ""
		cmd := m.rescan()
		if msg.err != nil {
			m.status = fmt.Sprintf("%s %s failed: %v", msg.op, msg.name, msg.err)
		} else {
			m.status = fmt.Sprintf("%s %s done", msg.op, msg.name)
		}
		return m, cmd

	case statusMsg:
		m.status = string(msg)

	case tea.KeyMsg:
		if msg.String() == "ctrl+c" {
			return m, tea.Quit
		}
		if m.filtering {
			return m.updateFilter(msg)
		}
		if m.committing {
			return m.updateCommit(msg)
		}
		return m.updateKeys(msg)
	}
	return m, nil
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
	return m, m.loadPreview()
}

func (m Model) updateCommit(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.committing, m.commitMsg = false, ""
	case "enter":
		if strings.TrimSpace(m.commitMsg) == "" {
			return m, nil
		}
		r := m.current()
		m.committing = false
		m.busy = "commit " + r.Name
		message := m.commitMsg
		m.commitMsg = ""
		return m, commitCmd(m.root, r.Name, message)
	case "backspace":
		if m.commitMsg != "" {
			_, size := utf8.DecodeLastRuneInString(m.commitMsg)
			m.commitMsg = m.commitMsg[:len(m.commitMsg)-size]
		}
	default:
		if msg.Type == tea.KeyRunes || msg.Type == tea.KeySpace {
			m.commitMsg += string(msg.Runes)
		}
	}
	return m, nil
}

func (m Model) updateKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	m.status = ""
	switch msg.String() {
	case "q", "esc":
		if m.filter != "" {
			m.filter = ""
			m.refilter()
			break
		}
		return m, tea.Quit

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
	case "r", "R":
		return m, m.rescan()
	case "F":
		if !m.fetching {
			m.fetching = true
			return m, fetchCmd(m.root, m.repos)
		}
	case "f":
		return m, m.gitOp("fetch", "fetch", "--all", "--quiet")
	case "p":
		return m, m.gitOp("pull", "pull", "--ff-only", "--quiet")
	case "P":
		return m, m.gitOp("push", "push", "--quiet")
	case "c":
		if r := m.current(); r != nil && r.Dirty() && m.busy == "" {
			m.committing, m.commitMsg = true, ""
		}
	case "w":
		if m.herdr.Workspace != "" && m.herdr.Available {
			m.workspaceOnly = !m.workspaceOnly
			m.refilter()
		}

	case "enter", "l":
		if m.current() != nil {
			return m, lazygitCmd(m.currentDir())
		}

	case "t":
		return m, m.jumpToHerdr()
	}
	return m, m.loadPreview()
}

// gitOp starts a git operation in the selected repo unless one is running.
func (m *Model) gitOp(op string, args ...string) tea.Cmd {
	r := m.current()
	if r == nil || m.busy != "" || r.Err != nil {
		return nil
	}
	m.busy = op + " " + r.Name
	return gitOpCmd(op, m.root, r.Name, args...)
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
