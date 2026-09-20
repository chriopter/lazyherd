package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/chriopter/lazyherd/internal/repo"
)

const (
	colChanges   = 4
	colGroup     = 2 // "⌂ " for repos of the current Herdr workspace
	colBranchMin = 8
	colSync      = 7
	gap          = 2
	minWidth     = 24
	minHeight    = 6
	branchMinW   = 44 // below this width the branch column is dropped

	// Screen rows above the first list entry: title line, top border, header.
	repoRowsTop = 3
)

func (m Model) View() string {
	if m.width == 0 {
		return ""
	}
	if m.width < minWidth || m.height < minHeight {
		return fmt.Sprintf("lazyherd needs at least %dx%d\n", minWidth, minHeight)
	}
	innerH := m.height - 4 // title, help, two borders
	style := activePaneStyle
	if m.filtering {
		style = paneStyle
	}
	line := lipgloss.NewStyle().MaxWidth(m.width)
	return line.Render(m.title()) + "\n" +
		style.Width(m.width-2).Height(innerH).MaxHeight(innerH+2).Render(m.table(m.width-4, innerH)) + "\n" +
		line.Render(m.help())
}

func (m Model) title() string {
	dirty := 0
	for _, r := range m.repos {
		if r.Dirty() {
			dirty++
		}
	}
	t := titleStyle.Render("lazyherd") + " " + dimStyle.Render(fmt.Sprintf("%d · ", len(m.repos))) + dirtyStyle.Render(fmt.Sprintf("%d dirty", dirty))
	switch {
	case m.loading:
		t += dimStyle.Render("  ⟳")
	case m.fetching:
		t += dimStyle.Render("  ⇣")
	case m.busy != "":
		t += dimStyle.Render("  ⟳ " + m.busy)
	}
	if m.herdr.Workspace != "" && m.herdr.Available {
		if m.workspaceOnly {
			t += "  " + filterStyle.Render("⌂ "+m.herdr.WorkspaceLabel())
		} else {
			t += "  " + dimStyle.Render("⌂ "+m.herdr.WorkspaceLabel()+" (all)")
		}
	}
	if m.filtering || m.filter != "" {
		t += "  " + filterStyle.Render("/"+m.filter+"▏")
	}
	if m.status != "" {
		t += "  " + dimStyle.Render(m.status)
	}
	return t
}

// groupWidth is the width of the workspace marker column, shown only when a
// Herdr workspace is known and the list mixes its repos with the others.
func (m Model) groupWidth() int {
	if m.herdr.Workspace != "" && m.herdr.Available && !m.workspaceOnly {
		return colGroup
	}
	return 0
}

// columns splits the free width between the name and branch columns: names
// get what the longest visible name needs, branches take the rest. Narrow
// lists drop the branch column.
func (m Model) columns(width int) (nameW, branchW int) {
	free := width - 1 - colChanges - m.groupWidth() - 2*gap - colSync - 1
	longest := 4
	for _, i := range m.visible {
		longest = max(longest, len(m.repos[i].Name))
	}
	if width < branchMinW {
		return max(min(longest, free), 6), 0
	}
	free -= gap
	nameW = max(min(longest, free-colBranchMin), 8)
	branchW = max(free-nameW, colBranchMin)
	return nameW, branchW
}

// repoWindow is the range of visible repo rows that fits the list height.
func (m Model) repoWindow() (start, count int) {
	count = max(m.height-5, 1)
	if m.cursor >= count {
		start = m.cursor - count + 1
	}
	return start, count
}

// repoRowAt maps a screen row to a visible repo index.
func (m Model) repoRowAt(y int) (int, bool) {
	start, count := m.repoWindow()
	i := start + y - repoRowsTop
	if y < repoRowsTop || i-start >= count || i >= len(m.visible) {
		return 0, false
	}
	return i, true
}

func (m Model) table(width, height int) string {
	nameW, branchW := m.columns(width)
	head := fmt.Sprintf(" %*s  %*s%-*s", colChanges, "CHG", m.groupWidth(), "", nameW, "REPO")
	if branchW > 0 {
		head += fmt.Sprintf("  %-*s", branchW, "BRANCH")
	}
	rows := []string{headerStyle.Render(head + "  SYNC")}
	start, count := m.repoWindow()
	for i := start; i < len(m.visible) && i-start < count; i++ {
		rows = append(rows, m.row(m.repos[m.visible[i]], i == m.cursor, nameW, branchW, width))
	}
	if len(m.visible) == 0 && !m.loading {
		rows = append(rows, dimStyle.Render("no repositories"))
	}
	return strings.Join(rows, "\n")
}

// highlight returns a style modifier that adds the selection background.
func highlight(selected bool) func(lipgloss.Style) lipgloss.Style {
	return func(s lipgloss.Style) lipgloss.Style {
		if selected {
			return s.Background(colorSelected).Bold(true)
		}
		return s
	}
}

func (m Model) row(r repo.Repo, selected bool, nameW, branchW, width int) string {
	st := highlight(selected)
	sp := func(n int) string { return st(lipgloss.NewStyle()).Render(strings.Repeat(" ", n)) }

	mark := sp(1)
	if selected {
		mark = st(branchStyle).Render("▶")
	}
	var changes, sync string
	switch {
	case r.Err != nil:
		changes = st(errorStyle).Render(fmt.Sprintf("%*s", colChanges, "!"))
		sync = st(errorStyle).Render("error")
	case r.Dirty():
		changes = st(dirtyStyle).Render(fmt.Sprintf("%*d", colChanges, len(r.Changes)))
	default:
		changes = st(cleanStyle).Render(fmt.Sprintf("%*s", colChanges, "·"))
	}
	if r.Err == nil {
		switch {
		case r.NoUpstream:
			sync = st(dimStyle).Render("no up")
		case r.Ahead > 0 && r.Behind > 0:
			sync = st(behindStyle).Render(fmt.Sprintf("↓%d", r.Behind)) + sp(1) + st(aheadStyle).Render(fmt.Sprintf("↑%d", r.Ahead))
		case r.Ahead > 0:
			sync = st(aheadStyle).Render(fmt.Sprintf("↑%d", r.Ahead))
		case r.Behind > 0:
			sync = st(behindStyle).Render(fmt.Sprintf("↓%d", r.Behind))
		default:
			sync = st(dimStyle).Render("✓")
		}
	}
	group := ""
	if gw := m.groupWidth(); gw > 0 {
		group = sp(gw)
		switch {
		case m.pinned(r.Name):
			group = st(dirtyStyle).Render("★") + sp(gw-1)
		case m.inWorkspace(r.Name):
			group = st(branchStyle).Render("⌂") + sp(gw-1)
		}
	}
	line := mark + changes + sp(gap) + group + st(textStyle).Render(fmt.Sprintf("%-*.*s", nameW, nameW, r.Name))
	if branchW > 0 {
		line += sp(gap) + st(branchStyle).Render(fmt.Sprintf("%-*.*s", branchW, branchW, r.Branch))
	}
	line += sp(gap) + sync
	if selected {
		line += sp(max(width-lipgloss.Width(line), 0))
	}
	return lipgloss.NewStyle().MaxWidth(width).Render(line)
}

func (m Model) help() string {
	k := func(key, desc string) string { return keyStyle.Render(key) + dimStyle.Render(" "+desc) }
	sep := dimStyle.Render(" · ")
	open := k("↵", "lazygit")
	if m.companionPane != "" {
		open = k("↵", "to lazygit")
	}
	keys := []string{open, k("p", "sync"), k("P", "sync listed"), k("/", "filter")}
	if m.herdr.Available {
		keys = append(keys, k("t", "herdr"))
	}
	if m.herdr.Workspace != "" && m.herdr.Available {
		if m.workspaceOnly {
			keys = append(keys, k("w", "all repos"))
		} else {
			keys = append(keys, k("w", "workspace"))
		}
		if r := m.current(); r != nil && m.pinned(r.Name) {
			keys = append(keys, k("␣", "unpin"))
		} else {
			keys = append(keys, k("␣", "pin"))
		}
	}
	keys = append(keys, k("q", "quit"))
	help := " " + strings.Join(keys, sep)
	if lipgloss.Width(help) > m.width {
		// Narrow pane: keys only, in the same order.
		short := []string{"↵", "p", "P", "/"}
		if m.herdr.Available {
			short = append(short, "t")
		}
		if m.herdr.Workspace != "" && m.herdr.Available {
			short = append(short, "w", "␣")
		}
		short = append(short, "q")
		for i, s := range short {
			short[i] = keyStyle.Render(s)
		}
		help = " " + strings.Join(short, sep)
	}
	return help
}

// tilde shortens a path under $HOME to ~/... for display.
func tilde(p string) string {
	if home, err := os.UserHomeDir(); err == nil && strings.HasPrefix(p, home+string(filepath.Separator)) {
		return "~" + p[len(home):]
	}
	return p
}
