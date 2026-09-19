package main

import (
	"os"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

type (
	reposMsg     []repo
	previewMsg   struct{ name, text string }
	rescanMsg    struct{}
	fetchDoneMsg struct{}
	herdrMsg     herdrState
	statusMsg    string
)

type model struct {
	root    string
	repos   []repo
	visible []int // indices into repos that pass the filters
	cursor  int   // index into visible
	preview map[string]string

	width, height int
	loading       bool
	fetching      bool
	filtering     bool   // typing into the name filter
	filter        string // current name filter
	wsOnly        bool   // only repos with a pane in the current Herdr workspace
	status        string // transient note in the title bar
	herdr         herdrState
}

func newModel(root string) model {
	return model{
		root:    root,
		preview: map[string]string{},
		loading: true,
		wsOnly:  os.Getenv("HERDR_WORKSPACE_ID") != "",
	}
}

func (m model) Init() tea.Cmd { return m.rescan() }

func (m model) rescan() tea.Cmd {
	return tea.Batch(scan(m.root), func() tea.Msg { return herdrMsg(loadHerdr(m.root)) })
}

func (m model) current() *repo {
	if m.cursor >= len(m.visible) {
		return nil
	}
	return &m.repos[m.visible[m.cursor]]
}

// applyFilter recomputes the visible rows and keeps the cursor on the same repo.
func (m *model) applyFilter() {
	keep := ""
	if r := m.current(); r != nil {
		keep = r.name
	}
	m.visible = m.visible[:0]
	f := strings.ToLower(m.filter)
	for i, r := range m.repos {
		if f != "" && !strings.Contains(strings.ToLower(r.name), f) {
			continue
		}
		if m.wsOnly && m.herdr.pane(r.name, m.herdr.curWS) == nil {
			continue
		}
		m.visible = append(m.visible, i)
	}
	m.cursor = 0
	for i, idx := range m.visible {
		if m.repos[idx].name == keep {
			m.cursor = i
		}
	}
}

// previewCmd loads the preview of the selected repo unless it is cached.
func (m model) previewCmd() tea.Cmd {
	r := m.current()
	if r == nil {
		return nil
	}
	if _, ok := m.preview[r.name]; ok {
		return nil
	}
	return loadPreview(m.root, r.name)
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height

	case reposMsg:
		m.repos, m.loading = msg, false
		m.applyFilter()
		return m, m.previewCmd()

	case herdrMsg:
		m.herdr = herdrState(msg)
		m.applyFilter()
		return m, m.previewCmd()

	case previewMsg:
		m.preview[msg.name] = msg.text

	case statusMsg:
		m.status = string(msg)

	case rescanMsg:
		m.loading = true
		m.preview = map[string]string{}
		return m, m.rescan()

	case fetchDoneMsg:
		m.fetching = false
		return m, func() tea.Msg { return rescanMsg{} }

	case tea.KeyMsg:
		if m.filtering {
			return m.updateFilter(msg)
		}
		return m.updateKeys(msg)
	}
	return m, nil
}

func (m model) updateFilter(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.filtering, m.filter = false, ""
	case "enter":
		m.filtering = false
	case "backspace":
		if m.filter != "" {
			m.filter = m.filter[:len(m.filter)-1]
		}
	default:
		if msg.Type == tea.KeyRunes {
			m.filter += string(msg.Runes)
		}
	}
	m.applyFilter()
	return m, m.previewCmd()
}

func (m model) updateKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	m.status = ""
	switch msg.String() {
	case "q", "esc", "ctrl+c":
		if m.filter != "" {
			m.filter = ""
			m.applyFilter()
			return m, m.previewCmd()
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
	case "r":
		return m, func() tea.Msg { return rescanMsg{} }
	case "f":
		if !m.fetching {
			m.fetching = true
			return m, fetchAll(m.root, m.repos)
		}
	case "w":
		if m.herdr.curWS != "" {
			m.wsOnly = !m.wsOnly
			m.applyFilter()
		}

	case "enter", "l":
		if r := m.current(); r != nil {
			return m, tea.ExecProcess(lazygit(filepath.Join(m.root, r.name)),
				func(error) tea.Msg { return rescanMsg{} })
		}

	case "t":
		return m, m.jumpToHerdr()
	}
	return m, m.previewCmd()
}

// jumpToHerdr focuses the Herdr tab holding the selected repo, or opens one.
func (m model) jumpToHerdr() tea.Cmd {
	r := m.current()
	if r == nil || !m.herdr.available {
		return nil
	}
	name, dir, ws := r.name, filepath.Join(m.root, r.name), m.herdr.curWS
	if p := m.herdr.pane(name, ws); p != nil {
		tab := p.TabID
		return func() tea.Msg {
			if err := focusTab(tab); err != nil {
				return statusMsg("focus failed: " + err.Error())
			}
			return statusMsg("→ " + name)
		}
	}
	return func() tea.Msg {
		if err := createTab(ws, dir, name); err != nil {
			return statusMsg("tab create failed: " + err.Error())
		}
		return rescanMsg{}
	}
}
