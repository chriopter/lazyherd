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

// preview is everything the right pane knows about one repository.
type preview struct {
	branch  string // "main...origin/main [ahead 1]"
	changes []repo.Change
	rows    []treeRow // changes laid out as a tree
	files   []int     // indices of file rows
	log     string
	err     error
}

type pane int

const (
	paneRepos pane = iota
	paneFiles
)

// Model is the whole application state.
type Model struct {
	root    string
	repos   []repo.Repo
	visible []int // indices into repos that pass the filters
	cursor  int   // index into visible
	herdr   herdr.State

	focus      pane
	fileCursor int // index into preview.files of the selected repo

	gen      int                 // bumped on every rescan; stale results are dropped
	previews map[string]preview  // cached by repo name for the current gen
	pending  map[string]struct{} // previews requested but not yet received
	diffs    map[string]string   // cached by repo name + path

	width, height int
	loading       bool
	fetching      bool   // fetch all in progress
	busy          string // running single-repo operation, e.g. "pull api"
	filtering     bool   // typing into the name filter
	filter        string // current name filter
	committing    bool   // commit dialog open
	commitMsg     string // subject typed into the commit dialog
	commitBody    string // description below the subject, from claude
	generating    bool   // claude is writing a commit message
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
		diffs:         map[string]string{},
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
	m.diffs = map[string]string{}
	return m.scanCmds()
}

func (m Model) current() *repo.Repo {
	if m.cursor >= len(m.visible) {
		return nil
	}
	return &m.repos[m.visible[m.cursor]]
}

func (m Model) currentDir() string { return filepath.Join(m.root, m.current().Name) }

// currentPreview is the cached preview of the selected repo, if loaded.
func (m Model) currentPreview() (preview, bool) {
	r := m.current()
	if r == nil {
		return preview{}, false
	}
	p, ok := m.previews[r.Name]
	return p, ok
}

// currentFile is the change selected in the file tree, if any.
func (m Model) currentFile() *repo.Change {
	p, ok := m.currentPreview()
	if !ok || m.fileCursor >= len(p.files) {
		return nil
	}
	return p.rows[p.files[m.fileCursor]].change
}

func diffKey(name, path string) string { return name + "\x00" + path }

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
	m.selectRepo(0)
	for i, idx := range m.visible {
		if m.repos[idx].Name == keep {
			m.selectRepo(i)
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

// selectRepo moves the repo cursor and resets the file selection.
func (m *Model) selectRepo(i int) {
	if i != m.cursor {
		m.fileCursor = 0
	}
	m.cursor = i
	if m.currentFile() == nil {
		m.focus = paneRepos
	}
}

// load requests whatever the right pane needs for the current selection.
func (m *Model) load() tea.Cmd {
	r := m.current()
	if r == nil {
		return nil
	}
	p, ok := m.previews[r.Name]
	if !ok {
		if _, pending := m.pending[r.Name]; pending {
			return nil
		}
		m.pending[r.Name] = struct{}{}
		return previewCmd(m.gen, m.root, r.Name)
	}
	if m.fileCursor >= len(p.files) {
		m.fileCursor = max(len(p.files)-1, 0)
	}
	if c := m.currentFile(); c != nil {
		if _, ok := m.diffs[diffKey(r.Name, c.Path)]; !ok {
			m.diffs[diffKey(r.Name, c.Path)] = "" // requested
			return diffCmd(m.gen, m.root, r.Name, *c)
		}
	}
	return nil
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
		return m, m.load()

	case herdrMsg:
		if msg.gen != m.gen {
			return m, nil
		}
		m.herdr = msg.state
		switch {
		case m.workspaceOnly && !m.herdr.Available:
			m.workspaceOnly = false
			m.status = "herdr not reachable, showing all repos"
		case m.workspaceOnly && !m.herdr.HasRepos(m.herdr.Workspace):
			m.workspaceOnly = false
			m.status = "no repos open in workspace " + m.herdr.WorkspaceLabel() + ", showing all"
		}
		m.refilter()
		return m, m.load()

	case previewMsg:
		if msg.gen != m.gen {
			return m, nil
		}
		delete(m.pending, msg.name)
		m.previews[msg.name] = msg.preview
		return m, m.load()

	case diffMsg:
		if msg.gen != m.gen {
			return m, nil
		}
		if msg.text == "" {
			msg.text = "(no diff)"
		}
		m.diffs[diffKey(msg.name, msg.path)] = msg.text

	case fetchDoneMsg:
		m.fetching = false
		cmd := m.rescan()
		if len(msg.failed) > 0 {
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

	case generatedMsg:
		m.generating = false
		switch {
		case msg.err != nil:
			m.status = "claude: " + msg.err.Error()
		case m.committing && m.current() != nil && m.current().Name == msg.name:
			m.commitMsg, m.commitBody = msg.subject, msg.body
		}

	case statusMsg:
		m.status = string(msg)

	case tea.MouseMsg:
		return m.updateMouse(msg)

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
		if m.focus == paneFiles {
			if next, cmd, handled := m.updateFileKeys(msg); handled {
				return next, cmd
			}
		}
		return m.updateKeys(msg)
	}
	return m, nil
}

func (m Model) updateMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	if msg.Action != tea.MouseActionPress || msg.Button != tea.MouseButtonLeft || m.committing {
		return m, nil
	}
	l := m.layout()
	if msg.X < l.leftW {
		if i, ok := m.repoRowAt(msg.Y); ok {
			m.selectRepo(i)
			m.focus = paneRepos
		}
		return m, m.load()
	}
	if i, ok := m.fileRowAt(msg.Y); ok {
		m.fileCursor = i
		m.focus = paneFiles
	}
	return m, m.load()
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
	return m, m.load()
}

func (m Model) updateCommit(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.committing, m.commitMsg, m.commitBody = false, "", ""
	case "ctrl+d":
		m.commitBody = ""
	case "tab":
		if !m.generating {
			m.generating = true
			return m, generateCmd(m.root, m.current().Name)
		}
	case "enter":
		if strings.TrimSpace(m.commitMsg) == "" {
			return m, nil
		}
		r := m.current()
		m.committing = false
		m.busy = "commit " + r.Name
		subject, body := m.commitMsg, m.commitBody
		m.commitMsg, m.commitBody = "", ""
		return m, commitCmd(m.root, r.Name, subject, body)
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

// updateFileKeys handles navigation inside the file tree; other keys fall
// through to the global bindings.
func (m Model) updateFileKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd, bool) {
	p, _ := m.currentPreview()
	switch msg.String() {
	case "down", "j":
		m.fileCursor = min(m.fileCursor+1, max(len(p.files)-1, 0))
	case "up", "k":
		m.fileCursor = max(m.fileCursor-1, 0)
	case "g", "home":
		m.fileCursor = 0
	case "G", "end":
		m.fileCursor = max(len(p.files)-1, 0)
	case "esc", "h", "left", "tab":
		m.focus = paneRepos
	default:
		return m, nil, false
	}
	return m, m.load(), true
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
		m.selectRepo(min(m.cursor+1, max(0, len(m.visible)-1)))
	case "up", "k":
		m.selectRepo(max(m.cursor-1, 0))
	case "g", "home":
		m.selectRepo(0)
	case "G", "end":
		m.selectRepo(max(0, len(m.visible)-1))
	case "l", "right", "tab":
		if m.currentFile() != nil {
			m.focus = paneFiles
		}

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
			m.committing, m.commitMsg, m.commitBody = true, "", ""
		}

	case "enter":
		if m.current() != nil {
			return m, lazygitCmd(m.currentDir())
		}

	case "t":
		return m, m.jumpToHerdr()
	}
	return m, m.load()
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
