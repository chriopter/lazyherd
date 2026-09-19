package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

var (
	cAccent = lipgloss.AdaptiveColor{Light: "#5A4FCF", Dark: "#8B7CFF"}
	cDim    = lipgloss.AdaptiveColor{Light: "#8A8A8A", Dark: "#6C6C6C"}
	cText   = lipgloss.AdaptiveColor{Light: "#222222", Dark: "#DDDDDD"}
	cWarn   = lipgloss.AdaptiveColor{Light: "#B26B00", Dark: "#F2B34E"}
	cGood   = lipgloss.AdaptiveColor{Light: "#1F7A3A", Dark: "#6CD483"}
	cBad    = lipgloss.AdaptiveColor{Light: "#B0303A", Dark: "#FF7B85"}
	cSelBg  = lipgloss.AdaptiveColor{Light: "#E6E2FF", Dark: "#33305A"}

	sTitle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#FFFFFF")).Background(cAccent).Padding(0, 1)
	sHead    = lipgloss.NewStyle().Bold(true).Foreground(cDim)
	sRow     = lipgloss.NewStyle().Foreground(cText)
	sDirty   = lipgloss.NewStyle().Foreground(cWarn).Bold(true)
	sClean   = lipgloss.NewStyle().Foreground(cGood)
	sDim     = lipgloss.NewStyle().Foreground(cDim)
	sAhead   = lipgloss.NewStyle().Foreground(cGood)
	sBehind  = lipgloss.NewStyle().Foreground(cBad)
	sBranch  = lipgloss.NewStyle().Foreground(cAccent)
	sKey     = lipgloss.NewStyle().Bold(true).Foreground(cAccent)
	sPane    = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(cDim).Padding(0, 1)
	sPaneAct = sPane.BorderForeground(cAccent)
	sPvTitle = lipgloss.NewStyle().Bold(true).Foreground(cAccent)
	sSection = lipgloss.NewStyle().Bold(true).Foreground(cDim).MarginTop(1)
	sFilter  = lipgloss.NewStyle().Foreground(cWarn).Bold(true)
)

const (
	colChanges   = 4
	colBranchMin = 8
	colSync      = 9
	gap          = 2
)

func (m model) View() string {
	if m.width == 0 {
		return ""
	}
	leftW := max(m.width*42/100, min(48, m.width))
	rightW := m.width - leftW
	paneH := m.height - 2 // title and help lines
	innerH := paneH - 2   // borders

	left := sPaneAct
	if m.filtering {
		left = sPane
	}
	return m.title() + "\n" +
		lipgloss.JoinHorizontal(lipgloss.Top,
			left.Width(leftW-2).Height(innerH).MaxHeight(paneH).Render(m.table(leftW-4, innerH)),
			sPane.Width(rightW-2).Height(innerH).MaxHeight(paneH).Render(m.previewPane(rightW-4, innerH)),
		) + "\n" + m.help()
}

func (m model) title() string {
	dirty := 0
	for _, r := range m.repos {
		if r.changes > 0 {
			dirty++
		}
	}
	t := sTitle.Render("lazyherd") + " " + sDim.Render(tilde(m.root)) + "  " +
		sDim.Render(fmt.Sprintf("%d repos · ", len(m.repos))) + sDirty.Render(fmt.Sprintf("%d dirty", dirty))
	switch {
	case m.loading:
		t += sDim.Render("  ⟳ scanning")
	case m.fetching:
		t += sDim.Render("  ⇣ fetching all")
	}
	if m.herdr.curWS != "" {
		if m.wsOnly {
			t += "  " + sFilter.Render("⌂ "+m.herdr.workspaceLabel())
		} else {
			t += "  " + sDim.Render("⌂ "+m.herdr.workspaceLabel()+" (all)")
		}
	}
	if m.filtering || m.filter != "" {
		t += "  " + sFilter.Render("/"+m.filter+"▏")
	}
	if m.status != "" {
		t += "  " + sDim.Render(m.status)
	}
	return t
}

func (m model) table(width, height int) string {
	nameW, branchW := m.columns(width)
	rows := []string{sHead.Render(fmt.Sprintf(" %*s  %-*s  %-*s  %s",
		colChanges, "CHG", nameW, "REPO", branchW, "BRANCH", "SYNC"))}

	listH := height - 1
	start := 0
	if m.cursor >= listH {
		start = m.cursor - listH + 1
	}
	for i := start; i < len(m.visible) && i-start < listH; i++ {
		rows = append(rows, m.row(m.repos[m.visible[i]], i == m.cursor, nameW, branchW, width))
	}
	if len(m.visible) == 0 && !m.loading {
		rows = append(rows, sDim.Render("no repositories"))
	}
	return strings.Join(rows, "\n")
}

// columns splits the free width between the name and branch columns: names
// get what the longest visible name needs, branches take the rest.
func (m model) columns(width int) (nameW, branchW int) {
	free := width - 1 - colChanges - 3*gap - colSync - 2
	longest := 4
	for _, i := range m.visible {
		longest = max(longest, len(m.repos[i].name))
	}
	nameW = max(min(longest, free-colBranchMin), 8)
	branchW = max(free-nameW, colBranchMin)
	return nameW, branchW
}

func (m model) row(r repo, selected bool, nameW, branchW, width int) string {
	st := func(s lipgloss.Style) lipgloss.Style {
		if selected {
			return s.Background(cSelBg).Bold(true)
		}
		return s
	}
	sp := func(n int) string { return st(lipgloss.NewStyle()).Render(strings.Repeat(" ", n)) }

	mark := sp(1)
	if selected {
		mark = st(sBranch).Render("▶")
	}
	changes := st(sClean).Render(fmt.Sprintf("%*s", colChanges, "·"))
	if r.changes > 0 {
		changes = st(sDirty).Render(fmt.Sprintf("%*d", colChanges, r.changes))
	}
	var sync string
	switch {
	case r.noUpstream:
		sync = st(sDim).Render("no up")
	case r.ahead > 0 && r.behind > 0:
		sync = st(sBehind).Render(fmt.Sprintf("↓%d", r.behind)) + sp(1) + st(sAhead).Render(fmt.Sprintf("↑%d", r.ahead))
	case r.ahead > 0:
		sync = st(sAhead).Render(fmt.Sprintf("↑%d", r.ahead))
	case r.behind > 0:
		sync = st(sBehind).Render(fmt.Sprintf("↓%d", r.behind))
	default:
		sync = st(sDim).Render("✓")
	}
	line := mark + changes + sp(gap) +
		st(sRow).Render(fmt.Sprintf("%-*.*s", nameW, nameW, r.name)) + sp(gap) +
		st(sBranch).Render(fmt.Sprintf("%-*.*s", branchW, branchW, r.branch)) + sp(gap) + sync
	if selected {
		line += sp(max(width-lipgloss.Width(line), 0))
	}
	return lipgloss.NewStyle().MaxWidth(width).Render(line)
}

func (m model) previewPane(width, height int) string {
	r := m.current()
	if r == nil {
		return ""
	}
	body := m.preview[r.name]
	if body == "" {
		body = sDim.Render("loading …")
	}
	text := sPvTitle.Render(r.name) + "  " + sDim.Render(tilde(filepath.Join(m.root, r.name))) + "\n" + body
	text = lipgloss.NewStyle().MaxWidth(width).Render(text)
	if lines := strings.Split(text, "\n"); len(lines) > height {
		text = strings.Join(lines[:height], "\n")
	}
	return text
}

func (m model) help() string {
	k := func(key, desc string) string { return sKey.Render(key) + sDim.Render(" "+desc) }
	keys := []string{k("↵", "lazygit"), k("/", "filter"), k("f", "fetch all"), k("r", "refresh")}
	if m.herdr.available {
		keys = append(keys, k("t", "herdr tab"))
	}
	if m.herdr.curWS != "" {
		keys = append(keys, k("w", "workspace/all"))
	}
	keys = append(keys, k("q", "quit"))
	return " " + strings.Join(keys, sDim.Render("  ·  "))
}

// tilde shortens a path under $HOME to ~/... for display.
func tilde(p string) string {
	if home, err := os.UserHomeDir(); err == nil && strings.HasPrefix(p, home+"/") {
		return "~" + p[len(home):]
	}
	return p
}
