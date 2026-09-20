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
	colSync      = 9
	gap          = 2
	minWidth     = 60
	minHeight    = 8

	// Screen rows above the first list entry: title line, top border, header.
	repoRowsTop = 3
	// Lines of the right pane above the first tree row: repo title, branch,
	// blank, CHANGES heading; plus title line and top border on screen.
	treeRowsTop = 2 + 4
)

// layout is the geometry shared by View and mouse hit-testing.
type layout struct {
	leftW, rightW int
	paneH, innerH int
}

func (m Model) layout() layout {
	leftW := max(m.width*42/100, 48)
	paneH := m.height - 2 // title and help lines
	return layout{leftW: leftW, rightW: m.width - leftW, paneH: paneH, innerH: paneH - 2}
}

func (m Model) View() string {
	if m.width == 0 {
		return ""
	}
	if m.width < minWidth || m.height < minHeight {
		return fmt.Sprintf("lazyherd needs at least %dx%d, got %dx%d\n", minWidth, minHeight, m.width, m.height)
	}
	l := m.layout()
	left, right := paneStyle, paneStyle
	if m.focus == paneRepos && !m.filtering {
		left = activePaneStyle
	}
	if m.focus == paneFiles {
		right = activePaneStyle
	}
	line := lipgloss.NewStyle().MaxWidth(m.width)
	body := lipgloss.JoinHorizontal(lipgloss.Top,
		left.Width(l.leftW-2).Height(l.innerH).MaxHeight(l.paneH).Render(m.table(l.leftW-4, l.innerH)),
		right.Width(l.rightW-2).Height(l.innerH).MaxHeight(l.paneH).Render(m.previewPane(l.rightW-4, l.innerH)),
	)
	if m.committing {
		body = lipgloss.Place(m.width, l.paneH, lipgloss.Center, lipgloss.Center, m.commitDialog())
	}
	return line.Render(m.title()) + "\n" + body + "\n" + line.Render(m.help())
}

// commitDialog is the box asking for a commit subject and description.
func (m Model) commitDialog() string {
	r := m.current()
	w := min(max(m.width*70/100, 84), m.width-4) // wide enough for 72-column text
	inner := w - 4
	active := lipgloss.NewStyle().Width(inner).Background(colorSelected).Foreground(colorText).Padding(0, 1)
	idle := lipgloss.NewStyle().Width(inner).Foreground(colorText).Padding(0, 1).
		Border(lipgloss.NormalBorder(), false, false, false, true).BorderForeground(colorDim)
	label := func(name string, n int) string {
		if m.commitField == n {
			return keyStyle.Render(name)
		}
		return headerStyle.Render(name)
	}
	style := func(n int) lipgloss.Style {
		if m.commitField == n {
			return active
		}
		return idle
	}

	subject, body := m.commitMsg, m.commitBody
	if m.generating {
		subject, body = "⟳ asking claude …", ""
	} else if m.commitField == 0 {
		subject += "▏"
	} else {
		body += "▏"
	}
	if lines := strings.Split(body, "\n"); len(lines) > 12 {
		body = "…\n" + strings.Join(lines[len(lines)-12:], "\n")
	}
	if body == "" {
		body = dimStyle.Render("optional")
	}
	parts := []string{
		previewTitleStyle.Render("Commit "+r.Name) + "  " +
			dimStyle.Render(fmt.Sprintf("%d changes, all will be staged", r.Changes)),
		"",
		label("SUBJECT", 0),
		style(0).Render(subject),
		"",
		label("DESCRIPTION", 1),
		style(1).Render(body),
		"",
		dimStyle.Render("enter commit · tab switch field · ctrl+g write with claude · esc cancel"),
	}
	return activePaneStyle.Width(w).Render(strings.Join(parts, "\n"))
}

// reflow joins hard-wrapped lines of each paragraph so the text can be
// wrapped to the dialog width; bullets stay on their own lines.
func reflow(text string) string {
	var out []string
	cur := ""
	flush := func() {
		if cur != "" {
			out = append(out, cur)
			cur = ""
		}
	}
	for _, line := range strings.Split(text, "\n") {
		t := strings.TrimSpace(line)
		switch {
		case t == "":
			flush()
			out = append(out, "")
		case strings.HasPrefix(t, "- ") || strings.HasPrefix(t, "* ") || strings.HasPrefix(t, "• "):
			flush()
			cur = "- " + strings.TrimSpace(t[2:])
		case cur == "":
			cur = t
		default:
			cur += " " + t
		}
	}
	flush()
	return strings.Join(out, "\n")
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
		t += dimStyle.Render("  ⇣ fetching")
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

// ---- left pane: repositories ----

// columns splits the free width between the name and branch columns: names
// get what the longest visible name needs, branches take the rest.
func (m Model) columns(width int) (nameW, branchW int) {
	free := width - 1 - colChanges - m.groupWidth() - 3*gap - colSync - 2
	longest := 4
	for _, i := range m.visible {
		longest = max(longest, len(m.repos[i].Name))
	}
	nameW = max(min(longest, free-colBranchMin), 8)
	branchW = max(free-nameW, colBranchMin)
	return nameW, branchW
}

// repoWindow is the range of visible repo rows that fits the list height.
func (m Model) repoWindow() (start, count int) {
	count = max(m.layout().innerH-1, 1)
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

// groupWidth is the width of the workspace marker column, shown only when a
// Herdr workspace is known and the list mixes its repos with the others.
func (m Model) groupWidth() int {
	if m.herdr.Workspace != "" && m.herdr.Available && !m.workspaceOnly {
		return colGroup
	}
	return 0
}

func (m Model) table(width, height int) string {
	nameW, branchW := m.columns(width)
	rows := []string{headerStyle.Render(fmt.Sprintf(" %*s  %*s%-*s  %-*s  %s",
		colChanges, "CHG", m.groupWidth(), "", nameW, "REPO", branchW, "BRANCH", "SYNC"))}
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
	group := ""
	if gw := m.groupWidth(); gw > 0 {
		group = sp(gw)
		if m.inWorkspace(r.Name) {
			group = st(branchStyle).Render("⌂") + sp(gw-1)
		}
	}
	line := mark + changes + sp(gap) + group +
		st(textStyle).Render(fmt.Sprintf("%-*.*s", nameW, nameW, r.Name)) + sp(gap) +
		st(branchStyle).Render(fmt.Sprintf("%-*.*s", branchW, branchW, r.Branch)) + sp(gap) + sync
	if selected {
		line += sp(max(width-lipgloss.Width(line), 0))
	}
	return lipgloss.NewStyle().MaxWidth(width).Render(line)
}

// ---- right pane: changes tree and diff ----

// treeHeight is how many tree rows the right pane shows for a preview.
func (m Model) treeHeight(p preview, innerH int) int {
	if len(p.rows) == 0 {
		return 0
	}
	return min(len(p.rows), max(innerH*45/100, 3))
}

// treeWindow is the range of tree rows shown, scrolled to the selected file.
func (m Model) treeWindow(p preview, innerH int) (start, count int) {
	count = m.treeHeight(p, innerH)
	if count == 0 {
		return 0, 0
	}
	sel := 0
	if m.fileCursor < len(p.files) {
		sel = p.files[m.fileCursor]
	}
	if sel >= count {
		start = sel - count + 1
	}
	return start, count
}

// fileRowAt maps a screen row to an index into the preview's file list.
func (m Model) fileRowAt(y int) (int, bool) {
	p, ok := m.currentPreview()
	if !ok {
		return 0, false
	}
	start, count := m.treeWindow(p, m.layout().innerH)
	row := start + y - treeRowsTop
	if y < treeRowsTop || row-start >= count || row >= len(p.rows) || p.rows[row].change == nil {
		return 0, false
	}
	for i, f := range p.files {
		if f == row {
			return i, true
		}
	}
	return 0, false
}

func (m Model) previewPane(width, height int) string {
	r := m.current()
	if r == nil {
		return ""
	}
	title := previewTitleStyle.Render(r.Name) + "  " + dimStyle.Render(tilde(m.currentDir()))
	p, ok := m.previews[r.Name]
	var lines []string
	switch {
	case r.Err != nil:
		lines = []string{title, errorStyle.Render(r.Err.Error())}
	case !ok:
		lines = []string{title, dimStyle.Render("loading …")}
	case p.err != nil:
		lines = []string{title, errorStyle.Render(p.err.Error())}
	default:
		lines = append(lines, title, dimStyle.Render(p.branch), "")
		if len(p.rows) == 0 {
			lines = append(lines, sectionStyle.Render("CHANGES"), cleanStyle.Render("clean"))
		} else {
			lines = append(lines, sectionStyle.Render(fmt.Sprintf("CHANGES (%d)", len(p.changes))))
			lines = append(lines, m.tree(p, width, height)...)
		}
		lines = append(lines, "")
		if c := m.currentFile(); c != nil {
			lines = append(lines, sectionStyle.Render("DIFF "+c.Path))
			d := m.diffs[diffKey(r.Name, c.Path)]
			if d == "" {
				d = dimStyle.Render("loading …")
			}
			lines = append(lines, strings.Split(d, "\n")...)
		} else {
			lines = append(lines, sectionStyle.Render("LOG"))
			lines = append(lines, strings.Split(p.log, "\n")...)
		}
	}
	if len(lines) > height {
		lines = lines[:max(height, 0)]
	}
	return lipgloss.NewStyle().MaxWidth(width).Render(strings.Join(lines, "\n"))
}

// tree renders the visible window of the change tree.
func (m Model) tree(p preview, width, height int) []string {
	start, count := m.treeWindow(p, height)
	sel := -1
	if m.fileCursor < len(p.files) {
		sel = p.files[m.fileCursor]
	}
	var out []string
	for i := start; i < len(p.rows) && i-start < count; i++ {
		row := p.rows[i]
		selected := i == sel
		st := highlight(selected && m.focus == paneFiles)
		sp := func(n int) string { return st(lipgloss.NewStyle()).Render(strings.Repeat(" ", n)) }
		indent := sp(row.depth * 2)
		var line string
		if row.change == nil {
			line = sp(1) + indent + sp(3) + st(dimStyle).Render(row.name)
		} else {
			code := row.change.Code()
			codeStyle := dimStyle
			switch {
			case row.change.Untracked:
				codeStyle = dimStyle
			case row.change.Staged != ' ' && row.change.Unstaged == ' ':
				codeStyle = aheadStyle // staged: green
			case row.change.Staged != ' ':
				codeStyle = dirtyStyle // partially staged
			default:
				codeStyle = behindStyle // unstaged: red
			}
			mark := sp(1)
			if selected {
				mark = st(branchStyle).Render("▶")
			}
			line = mark + indent + st(codeStyle).Render(code) + sp(1) + st(textStyle).Render(row.name)
		}
		if selected && m.focus == paneFiles {
			line += sp(max(width-lipgloss.Width(line), 0))
		}
		out = append(out, lipgloss.NewStyle().MaxWidth(width).Render(line))
	}
	if start+count < len(p.rows) {
		out = append(out, dimStyle.Render(fmt.Sprintf("  … %d more", len(p.rows)-start-count)))
	}
	return out
}

func (m Model) help() string {
	k := func(key, desc string) string { return keyStyle.Render(key) + dimStyle.Render(" "+desc) }
	sep := dimStyle.Render(" · ")
	if m.committing {
		commit := k("↵", "commit")
		if m.commitField == 1 {
			commit = k("alt+↵", "commit")
		}
		return " " + strings.Join([]string{commit, k("tab", "switch field"), k("ctrl+g", "write with claude"), k("esc", "cancel")}, sep)
	}
	if m.focus == paneFiles {
		return " " + strings.Join([]string{k("j/k", "file"), k("esc", "back"), k("↵", "lazygit"), k("c", "commit"), k("q", "quit")}, sep)
	}
	keys := []string{k("↵", "lazygit"), k("l", "files"), k("c", "commit"), k("p", "pull"), k("P", "push"), k("/", "filter")}
	if m.herdr.Available {
		keys = append(keys, k("t", "herdr"))
	}
	if m.herdr.Workspace != "" && m.herdr.Available {
		keys = append(keys, k("w", "workspace"))
	}
	keys = append(keys, k("q", "quit"))
	return " " + strings.Join(keys, sep)
}

// tilde shortens a path under $HOME to ~/... for display.
func tilde(p string) string {
	if home, err := os.UserHomeDir(); err == nil && strings.HasPrefix(p, home+string(filepath.Separator)) {
		return "~" + p[len(home):]
	}
	return p
}
