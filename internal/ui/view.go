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
	colBranchMin = 8
	colSync      = 9
	gap          = 2
	minWidth     = 60
	minHeight    = 8
)

func (m Model) View() string {
	if m.width == 0 {
		return ""
	}
	if m.width < minWidth || m.height < minHeight {
		return fmt.Sprintf("lazyherd needs at least %dx%d, got %dx%d\n", minWidth, minHeight, m.width, m.height)
	}
	leftW := max(m.width*42/100, 48)
	rightW := m.width - leftW
	paneH := m.height - 2 // title and help lines
	innerH := paneH - 2   // borders

	left := activePaneStyle
	if m.filtering {
		left = paneStyle
	}
	line := lipgloss.NewStyle().MaxWidth(m.width)
	body := lipgloss.JoinHorizontal(lipgloss.Top,
		left.Width(leftW-2).Height(innerH).MaxHeight(paneH).Render(m.table(leftW-4, innerH)),
		paneStyle.Width(rightW-2).Height(innerH).MaxHeight(paneH).Render(m.previewPane(rightW-4, innerH)),
	)
	if m.committing {
		body = lipgloss.Place(m.width, paneH, lipgloss.Center, lipgloss.Center, m.commitDialog())
	}
	return line.Render(m.title()) + "\n" + body + "\n" + line.Render(m.help())
}

// commitDialog is the small box asking for a commit message.
func (m Model) commitDialog() string {
	r := m.current()
	w := min(max(m.width*60/100, 50), m.width-4)
	input := m.commitMsg + "▏"
	body := previewTitleStyle.Render("Commit "+r.Name) + "  " +
		dimStyle.Render(fmt.Sprintf("%d changes, all will be staged", r.Changes)) + "\n\n" +
		lipgloss.NewStyle().MaxWidth(w-4).Render(input) + "\n\n" +
		dimStyle.Render("enter commit · esc cancel")
	return activePaneStyle.Width(w).Render(body)
}

func (m Model) title() string {
	dirty := 0
	for _, r := range m.repos {
		if r.Dirty() {
			dirty++
		}
	}
	t := titleStyle.Render("lazyherd") + " " + dimStyle.Render(tilde(m.root)) + "  " +
		dimStyle.Render(fmt.Sprintf("%d repos · ", len(m.repos))) + dirtyStyle.Render(fmt.Sprintf("%d dirty", dirty))
	switch {
	case m.loading:
		t += dimStyle.Render("  ⟳ scanning")
	case m.fetching:
		t += dimStyle.Render("  ⇣ fetching all")
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

// columns splits the free width between the name and branch columns: names
// get what the longest visible name needs, branches take the rest.
func (m Model) columns(width int) (nameW, branchW int) {
	free := width - 1 - colChanges - 3*gap - colSync - 2
	longest := 4
	for _, i := range m.visible {
		longest = max(longest, len(m.repos[i].Name))
	}
	nameW = max(min(longest, free-colBranchMin), 8)
	branchW = max(free-nameW, colBranchMin)
	return nameW, branchW
}

func (m Model) table(width, height int) string {
	nameW, branchW := m.columns(width)
	rows := []string{headerStyle.Render(fmt.Sprintf(" %*s  %-*s  %-*s  %s",
		colChanges, "CHG", nameW, "REPO", branchW, "BRANCH", "SYNC"))}

	listH := max(height-1, 1)
	start := 0
	if m.cursor >= listH {
		start = m.cursor - listH + 1
	}
	for i := start; i < len(m.visible) && i-start < listH; i++ {
		rows = append(rows, m.row(m.repos[m.visible[i]], i == m.cursor, nameW, branchW, width))
	}
	if len(m.visible) == 0 && !m.loading {
		rows = append(rows, dimStyle.Render("no repositories"))
	}
	return strings.Join(rows, "\n")
}

func (m Model) row(r repo.Repo, selected bool, nameW, branchW, width int) string {
	st := func(s lipgloss.Style) lipgloss.Style {
		if selected {
			return s.Background(colorSelected).Bold(true)
		}
		return s
	}
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
		changes = st(dirtyStyle).Render(fmt.Sprintf("%*d", colChanges, r.Changes))
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
	line := mark + changes + sp(gap) +
		st(textStyle).Render(fmt.Sprintf("%-*.*s", nameW, nameW, r.Name)) + sp(gap) +
		st(branchStyle).Render(fmt.Sprintf("%-*.*s", branchW, branchW, r.Branch)) + sp(gap) + sync
	if selected {
		line += sp(max(width-lipgloss.Width(line), 0))
	}
	return lipgloss.NewStyle().MaxWidth(width).Render(line)
}

func (m Model) previewPane(width, height int) string {
	r := m.current()
	if r == nil {
		return ""
	}
	var body string
	if r.Err != nil {
		body = errorStyle.Render(r.Err.Error())
	} else if p, ok := m.previews[r.Name]; ok {
		body = sectionStyle.Render("STATUS") + "\n" + p.status + "\n" +
			sectionStyle.Render("LOG") + "\n" + p.log
	} else {
		body = dimStyle.Render("loading …")
	}
	text := previewTitleStyle.Render(r.Name) + "  " + dimStyle.Render(tilde(m.currentDir())) + "\n" + body
	text = lipgloss.NewStyle().MaxWidth(width).Render(text)
	if lines := strings.Split(text, "\n"); len(lines) > height {
		text = strings.Join(lines[:max(height, 0)], "\n")
	}
	return text
}

func (m Model) help() string {
	k := func(key, desc string) string { return keyStyle.Render(key) + dimStyle.Render(" "+desc) }
	if m.committing {
		return " " + k("↵", "commit") + dimStyle.Render(" · ") + k("esc", "cancel")
	}
	keys := []string{k("↵", "lazygit"), k("c", "commit"), k("p", "pull"), k("P", "push"), k("f", "fetch"), k("F", "fetch all"), k("R", "refresh"), k("/", "filter")}
	if m.herdr.Available {
		keys = append(keys, k("t", "herdr"))
	}
	if m.herdr.Workspace != "" && m.herdr.Available {
		keys = append(keys, k("w", "workspace"))
	}
	keys = append(keys, k("q", "quit"))
	return " " + strings.Join(keys, dimStyle.Render(" · "))
}

// tilde shortens a path under $HOME to ~/... for display.
func tilde(p string) string {
	if home, err := os.UserHomeDir(); err == nil && strings.HasPrefix(p, home+string(filepath.Separator)) {
		return "~" + p[len(home):]
	}
	return p
}
