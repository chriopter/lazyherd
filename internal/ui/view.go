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
	authorMaxW   = 10 // timeline author column

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
	return m.styledFrame(index, title, style.Render(title), subtitle, footer, body, width, height, style)
}

// styledFrame is frame with a title that brings its own styling, such as
// tabs; title is the same text unstyled.
func (m Model) styledFrame(index int, title, styledTitle, subtitle, footer string, body []string, width, height int, style lipgloss.Style) string {
	r := m.theme.frame
	h, v := string(r[0]), string(r[1])
	inner := width - 2

	prefix := fmt.Sprintf("%s[%d]%s", h, index, h)
	rest := ""
	used := lipgloss.Width(prefix) + lipgloss.Width(title)
	if subtitle != "" && used+lipgloss.Width(subtitle)+6 <= inner {
		rest = strings.Repeat(h, inner-used-lipgloss.Width(subtitle)-4) + subtitle + strings.Repeat(h, 4)
	}
	rest += strings.Repeat(h, max(inner-used-lipgloss.Width(rest), 0))
	top := style.Render(string(r[2])+prefix) + styledTitle + style.Render(rest+string(r[3]))

	bottom := strings.Repeat(h, inner)
	if footer != "" && lipgloss.Width(footer)+1 <= inner {
		bottom = strings.Repeat(h, inner-lipgloss.Width(footer)-1) + footer + h
	}
	bottom = string(r[4]) + bottom + string(r[5])

	rows := make([]string, 0, height)
	rows = append(rows, top)
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
	rows := m.rows
	if m.timeline {
		rows = m.changeRows
	}
	footer := ""
	if n := m.rowCount(); n > 0 {
		footer = fmt.Sprintf("%d of %d", m.cursorPos()+1, n)
	}
	title, styled := m.tabsTitle(m.border(true))
	return m.styledFrame(1, title, styled, subtitle, footer, rows(m.width-2), m.width, height, m.border(true))
}

// tabNames are the list's tabs, in order; the second is the Changes tab.
var tabNames = [2]string{"Repos", "Changes"}

const (
	tabSep    = " - "
	tabsLeft  = 6 // screen column where the tabs start: corner, "─[1]─"
	tabsWidth = 5 + 3 + 7
)

// tabsTitle renders the tabs the way lazygit titles a view with tabs: the
// open one highlighted, the other plain. A list too narrow for both shows
// only the open one.
func (m Model) tabsTitle(style lipgloss.Style) (title, styled string) {
	suffix := ""
	if m.filter != "" {
		suffix = " (filtered)"
	}
	open := 0
	if m.timeline {
		open = 1
	}
	if !m.tabsShown() {
		title = tabNames[open] + suffix
		if tabsLeft-1+len(title) > m.width-2 {
			title = tabNames[open]
		}
		return title, style.Render(title)
	}
	for i, name := range tabNames {
		if i > 0 {
			title += tabSep
			styled += style.Render(tabSep)
		}
		title += name
		if i == open {
			styled += style.Bold(true).Render(name)
		} else {
			styled += m.theme.text.Render(name)
		}
	}
	return title + suffix, styled + style.Render(suffix)
}

// tabsShown reports whether the list is wide enough to show both tabs.
func (m Model) tabsShown() bool {
	return m.width-2 >= tabsLeft-1+tabsWidth+len(" (filtered)")
}

// tabAt maps a click on the list's top border to a tab.
func (m Model) tabAt(x, y int) (changes, ok bool) {
	if y != statusPanelH || !m.tabsShown() {
		return false, false
	}
	switch x -= tabsLeft; {
	case x >= 0 && x < len(tabNames[0]):
		return false, true
	case x >= len(tabNames[0])+len(tabSep) && x < tabsWidth:
		return true, true
	}
	return false, false
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
	if cur := m.cursorPos(); cur >= count {
		start = cur - count + 1
	}
	return start, count
}

// repoRowAt maps a screen row to a visible repo index.
func (m Model) repoRowAt(y int) (int, bool) {
	start, count := m.repoWindow()
	i := start + y - repoRowsTop
	if y < repoRowsTop || i-start >= count || i >= m.rowCount() {
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

// changeRows renders the Changes tab: uncommitted work first, then commits
// with age, repo and subject, and the author at the right when the list is
// wide enough.
func (m Model) changeRows(width int) []string {
	repoW, authorW := 4, 0
	for _, c := range m.changes {
		repoW = max(repoW, len(c.Repo))
		authorW = max(authorW, len([]rune(firstName(c.Author))))
	}
	repoW = min(repoW, max(width/4, 8))
	if width < branchMinW {
		authorW = 0
	}
	authorW = min(authorW, authorMaxW)
	start, count := m.repoWindow()
	var rows []string
	for i := start; i < len(m.changes) && i-start < count; i++ {
		rows = append(rows, m.changeRow(m.changes[i], i == m.changeCursor, repoW, authorW, width))
	}
	if len(m.changes) == 0 {
		rows = append(rows, dim.Render(" no changes"))
	}
	return rows
}

func (m Model) changeRow(c repo.Commit, selected bool, repoW, authorW, width int) string {
	sp := func(n int) string {
		return m.style(lipgloss.NewStyle(), selected).Render(strings.Repeat(" ", n))
	}
	worktree := c.Hash == repo.WorkTree
	age := m.style(dim, selected).Render(fmt.Sprintf("%*s", ageW, repo.Ago(m.now, c.Time)))
	if worktree {
		age = m.style(m.theme.unstaged, selected).Render(fmt.Sprintf("%*s", ageW, "●"))
	}
	line := sp(1) + age + sp(1)
	group := ""
	if gw := m.groupWidth(); gw > 0 {
		group = sp(gw)
		switch {
		case m.pinned(c.Repo):
			group = m.style(yellow, selected).Render("★") + sp(gw-1)
		case m.inWorkspace(c.Repo):
			group = m.style(cyan, selected).Render("⌂") + sp(gw-1)
		}
	}
	line += group + m.style(cyan, selected).Render(fmt.Sprintf("%-*.*s", repoW, repoW, c.Repo)) + sp(1)
	subjectW := width - lipgloss.Width(line)
	if authorW > 0 {
		subjectW -= authorW + 1
	}
	if worktree {
		summary := m.worktreeSummary(c.Repo, selected)
		line += summary + sp(max(subjectW-lipgloss.Width(summary), 0))
	} else {
		line += m.style(m.theme.text, selected).Render(fmt.Sprintf("%-*s", max(subjectW, 0), truncate(c.Subject, subjectW)))
	}
	if authorW > 0 {
		line += sp(1) + m.style(green, selected).Render(fmt.Sprintf("%-*s", authorW, truncate(firstName(c.Author), authorW)))
	}
	if selected {
		line += sp(max(width-lipgloss.Width(line), 0))
	}
	return lipgloss.NewStyle().MaxWidth(width).Render(line)
}

// worktreeSummary counts a repo's uncommitted files the way lazygit colors
// them: staged in green, unstaged and untracked in the unstaged color.
func (m Model) worktreeSummary(name string, selected bool) string {
	var staged, unstaged, untracked int
	for _, r := range m.repos {
		if r.Name != name {
			continue
		}
		for _, c := range r.Changes {
			switch {
			case c.Untracked:
				untracked++
			default:
				if c.Staged != '.' {
					staged++
				}
				if c.Unstaged != '.' {
					unstaged++
				}
			}
		}
	}
	var parts []string
	for _, p := range []struct {
		n     int
		label string
		style lipgloss.Style
	}{{staged, "staged", green}, {unstaged, "unstaged", m.theme.unstaged}, {untracked, "untracked", m.theme.unstaged}} {
		if p.n > 0 {
			parts = append(parts, m.style(p.style, selected).Render(fmt.Sprintf("%d %s", p.n, p.label)))
		}
	}
	return strings.Join(parts, m.style(dim, selected).Render(" · "))
}

// truncate cuts s to width runes, ending in an ellipsis when cut.
func truncate(s string, width int) string {
	r := []rune(s)
	if width <= 0 || len(r) <= width {
		return s
	}
	return string(r[:width-1]) + "…"
}

// firstName shortens an author to the part before the first space.
func firstName(author string) string {
	name, _, _ := strings.Cut(author, " ")
	return name
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
	opts := []opt{{"Open", "<enter>"}}
	if m.timeline {
		opts = append(opts, opt{"Repos", "<tab>"})
	} else {
		opts = append(opts, opt{"Changes", "<tab>"})
	}
	opts = append(opts, opt{"Sync", "p"}, opt{"Sync listed", "P"}, opt{"Filter", "/"}, opt{"Herdr tab", "t"})
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
