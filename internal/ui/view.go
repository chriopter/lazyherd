package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/chriopter/lazyherd/internal/repo"
)

// Layout, top to bottom: status panel (3 rows), repos panel, options line.
const (
	statusPanelH = 3
	minWidth     = 24
	minHeight    = 8
	ageMinW      = 36 // below this width the last-commit column is dropped
	branchMinW   = 52 // below this width the branch column is dropped
	ageW         = 3  // "12M"

	// Screen row of the first repo line: status panel plus the list's top border.
	repoRowsTop = statusPanelH + 1
)

func (m Model) View() string {
	if m.width == 0 {
		return ""
	}
	if m.width < minWidth || m.height < minHeight {
		msg := fmt.Sprintf("lazyherd needs %dx%d", minWidth, minHeight)
		return lipgloss.NewStyle().MaxWidth(m.width).Render(msg) + "\n"
	}
	listH := m.height - statusPanelH - 1 // options line
	line := lipgloss.NewStyle().MaxWidth(m.width)
	return m.statusPanel() + "\n" + m.reposPanel(listH) + "\n" + line.Render(m.bottomLine())
}

// border picks the frame style the way lazygit does: green when active,
// cyan while searching, plain otherwise.
func (m Model) border(active bool) lipgloss.Style {
	switch {
	case active && m.filtering:
		return m.theme.searchingBorder
	case active:
		return m.theme.activeBorder
	}
	return m.theme.inactiveBorder
}

// frame draws a gocui-style box: the title sits in the top border after a
// "[n]" prefix, the subtitle at the top right, the footer at the bottom right.
func (m Model) frame(index int, title, subtitle, footer string, body []string, width, height int, style lipgloss.Style) string {
	r := m.theme.frame
	h, v := string(r[0]), string(r[1])
	inner := width - 2

	top := fmt.Sprintf("%s[%d]%s%s", h, index, h, title)
	if subtitle != "" && lipgloss.Width(top)+lipgloss.Width(subtitle)+6 <= inner {
		top += strings.Repeat(h, inner-lipgloss.Width(top)-lipgloss.Width(subtitle)-4) + subtitle + strings.Repeat(h, 4)
	}
	top = string(r[2]) + top + strings.Repeat(h, max(inner-lipgloss.Width(top), 0)) + string(r[3])

	bottom := strings.Repeat(h, inner)
	if footer != "" && lipgloss.Width(footer)+1 <= inner {
		bottom = strings.Repeat(h, inner-lipgloss.Width(footer)-1) + footer + h
	}
	bottom = string(r[4]) + bottom + string(r[5])

	rows := make([]string, 0, height)
	rows = append(rows, style.Render(top))
	for i := 0; i < height-2; i++ {
		content := ""
		if i < len(body) {
			content = body[i]
		}
		content = lipgloss.NewStyle().MaxWidth(inner).Render(content)
		content += strings.Repeat(" ", max(inner-lipgloss.Width(content), 0))
		rows = append(rows, style.Render(v)+content+style.Render(v))
	}
	rows = append(rows, style.Render(bottom))
	return strings.Join(rows, "\n")
}

// statusPanel mirrors lazygit's status view for the selected repo:
// "✓ repo → branch", with the sync state in front.
func (m Model) statusPanel() string {
	body := ""
	if r := m.current(); r != nil {
		body = m.syncState(*r, false)
		if body != "" {
			body += " "
		}
		body += r.Name + " → " + m.branchName(*r, false)
	} else if m.activity == "scanning" {
		body = dim.Render("scanning " + spinner[m.spin%len(spinner)])
	}
	return m.frame(0, "Status", "", "", []string{body}, m.width, statusPanelH, m.border(false))
}

func (m Model) reposPanel(height int) string {
	subtitle := "⌂ " + m.herdr.WorkspaceLabel()
	if !m.workspaceOnly {
		subtitle += " (all)"
	}
	title := "Repos"
	if m.filter != "" {
		title += " (filtered)"
	}
	footer := ""
	if len(m.visible) > 0 {
		footer = fmt.Sprintf("%d of %d", m.cursor+1, len(m.visible))
	}
	return m.frame(1, title, subtitle, footer, m.rows(m.width-2), m.width, height, m.border(true))
}

// syncState renders the branch status the way lazygit's BranchStatus does.
func (m Model) syncState(r repo.Repo, selected bool) string {
	switch {
	case r.Err != nil:
		return m.style(red, selected).Render("!")
	case r.NoUpstream:
		return ""
	case r.Ahead == 0 && r.Behind == 0:
		return m.style(green, selected).Render("✓")
	case r.Ahead > 0 && r.Behind > 0:
		return m.style(yellow, selected).Render(fmt.Sprintf("↓%d↑%d", r.Behind, r.Ahead))
	case r.Behind > 0:
		return m.style(yellow, selected).Render(fmt.Sprintf("↓%d", r.Behind))
	default:
		return m.style(yellow, selected).Render(fmt.Sprintf("↑%d", r.Ahead))
	}
}

func (m Model) branch(r repo.Repo) string {
	name := r.Branch
	if m.theme.icons != nil {
		icon := m.theme.icons.branch
		if strings.HasPrefix(name, "@") {
			icon = m.theme.icons.detachedHead
		}
		name = icon + " " + name
	}
	return name
}

func (m Model) branchName(r repo.Repo, selected bool) string {
	return m.style(m.theme.text, selected).Render(m.branch(r))
}

func (m Model) style(style lipgloss.Style, selected bool) lipgloss.Style {
	if selected {
		return style.Inherit(m.theme.selectedBg).Bold(true)
	}
	return style
}

// columns splits the free width between the name and branch columns: names
// get what the longest visible name needs, branches take the rest. Narrow
// lists drop the branch column.
// The last-commit age comes before the branch: narrow lists drop the branch
// first, then the age.
func (m Model) columns(width int) (nameW, branchW int, age bool) {
	const countW, syncW = 3, 6
	free := width - 1 - countW - 1 - m.groupWidth() - 1 - syncW
	longest := 4
	for _, i := range m.visible {
		longest = max(longest, len(m.repos[i].Name))
	}
	if age = width >= ageMinW; age {
		free -= ageW + 1
	}
	if width < branchMinW {
		return max(min(longest, free), 6), 0, age
	}
	free--
	nameW = max(min(longest, free-8), 8)
	branchW = max(free-nameW, 8)
	return nameW, branchW, age
}

// groupWidth is the width of the workspace marker column, shown only when
// the list mixes the workspace's repos with the others.
func (m Model) groupWidth() int {
	if !m.workspaceOnly {
		return 2
	}
	return 0
}

// repoWindow is the range of visible repo rows that fits the list height.
func (m Model) repoWindow() (start, count int) {
	count = max(m.height-statusPanelH-3, 1) // list borders and options line
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

func (m Model) rows(width int) []string {
	nameW, branchW, age := m.columns(width)
	start, count := m.repoWindow()
	var rows []string
	for i := start; i < len(m.visible) && i-start < count; i++ {
		rows = append(rows, m.row(m.repos[m.visible[i]], i == m.cursor, nameW, branchW, age, width))
	}
	if len(m.visible) == 0 && m.activity != "scanning" {
		rows = append(rows, dim.Render(" no repositories"))
	}
	return rows
}

func (m Model) row(r repo.Repo, selected bool, nameW, branchW int, age bool, width int) string {
	sp := func(n int) string {
		return m.style(lipgloss.NewStyle(), selected).Render(strings.Repeat(" ", n))
	}

	// Change count in lazygit's unstaged color, staged-only changes in green.
	count := sp(3)
	if r.Err == nil && r.Dirty() {
		style := green
		for _, c := range r.Changes {
			if c.Untracked || c.Unstaged != '.' {
				style = m.theme.unstaged
				break
			}
		}
		count = m.style(style, selected).Render(fmt.Sprintf("%3d", len(r.Changes)))
	}
	group := ""
	if gw := m.groupWidth(); gw > 0 {
		group = sp(gw)
		switch {
		case m.pinned(r.Name):
			group = m.style(yellow, selected).Render("★") + sp(gw-1)
		case m.inWorkspace(r.Name):
			group = m.style(cyan, selected).Render("⌂") + sp(gw-1)
		}
	}
	line := sp(1) + count + sp(1) + group + m.style(m.theme.text, selected).Render(fmt.Sprintf("%-*.*s", nameW, nameW, r.Name))
	if branchW > 0 {
		branch := r.Branch
		if m.theme.icons != nil {
			branch = m.theme.icons.branch + " " + branch
		}
		line += sp(1) + m.style(m.theme.text, selected).Render(fmt.Sprintf("%-*.*s", branchW, branchW, branch))
	}
	if age {
		line += sp(1) + m.style(dim, selected).Render(fmt.Sprintf("%*s", ageW, repo.Ago(m.now, r.Committed)))
	}
	line += sp(1) + m.syncState(r, selected)
	if selected {
		line += sp(max(width-lipgloss.Width(line), 0))
	}
	return lipgloss.NewStyle().MaxWidth(width).Render(line)
}

// bottomLine is lazygit's options bar: "Desc: key | Desc: key", with the
// filter prompt taking over while typing and the version at the right.
func (m Model) bottomLine() string {
	left := ""
	switch {
	case m.filtering:
		left = "Filter: " + m.filter
	case m.status != "":
		left = yellow.Render(m.status)
	default:
		left = m.options()
	}
	right := ""
	switch {
	case strings.HasPrefix(m.activity, "syncing"):
		right = cyan.Render(m.activity + " " + spinner[m.spin%len(spinner)])
	case m.activity == "fetching":
		right = dim.Render("fetching " + spinner[m.spin%len(spinner)])
	default:
		right = dim.Render("lazyherd " + m.version)
	}
	gap := m.width - lipgloss.Width(left) - lipgloss.Width(right) - 1
	if gap < 1 {
		return " " + left
	}
	return " " + left + strings.Repeat(" ", gap) + right
}

func (m Model) options() string {
	type opt struct{ desc, key string }
	opts := []opt{{"Open", "<enter>"}, {"Sync", "p"}, {"Sync listed", "P"}, {"Filter", "/"}, {"Herdr tab", "t"}}
	if m.workspaceOnly {
		opts = append(opts, opt{"All repos", "w"})
	} else {
		opts = append(opts, opt{"Workspace", "w"})
	}
	if r := m.current(); r != nil && m.pinned(r.Name) {
		opts = append(opts, opt{"Unpin", "<space>"})
	} else {
		opts = append(opts, opt{"Pin", "<space>"})
	}
	opts = append(opts, opt{"Quit", "q"})

	width := m.width - 2
	sep := " | "
	more := sep + "…"
	var b strings.Builder
	length := 0
	for i, o := range opts {
		text := o.desc + ": " + o.key
		// Keep room for the ellipsis unless this is the last entry.
		need := len(sep) + lipgloss.Width(text)
		if i < len(opts)-1 {
			need += len(more)
		}
		if i > 0 && length+need > width {
			b.WriteString(m.theme.options.Render(more))
			break
		}
		if i > 0 {
			b.WriteString(m.theme.options.Render(sep))
			length += len(sep)
		}
		b.WriteString(m.theme.options.Render(text))
		length += lipgloss.Width(text)
	}
	return b.String()
}
