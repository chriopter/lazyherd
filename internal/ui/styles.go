package ui

import (
	"github.com/charmbracelet/lipgloss"

	"github.com/chriopter/lazyherd/internal/lazygit"
)

// theme is lazyherd's look, derived from the user's lazygit configuration so
// both panes match. Colors are the terminal's ANSI palette unless the user
// configured others.
type theme struct {
	frame           [6]rune
	icons           *lazygit.Icons
	activeBorder    lipgloss.Style
	inactiveBorder  lipgloss.Style
	searchingBorder lipgloss.Style
	options         lipgloss.Style
	selectedBg      lipgloss.Style
	unstaged        lipgloss.Style
	text            lipgloss.Style
}

func newTheme(cfg lazygit.Config) theme {
	t := cfg.Gui.Theme
	return theme{
		frame:           lazygit.FrameRunes(cfg.Gui.Border),
		icons:           lazygit.IconsFor(cfg.Gui.NerdFontsVersion),
		activeBorder:    lazygit.Style(t.ActiveBorderColor, false),
		inactiveBorder:  lazygit.Style(t.InactiveBorderColor, false),
		searchingBorder: lazygit.Style(t.SearchingActiveBorderColor, false),
		options:         lazygit.Style(t.OptionsTextColor, false),
		selectedBg:      lazygit.Style(t.SelectedLineBgColor, true),
		unstaged:        lazygit.Style(t.UnstagedChangesColor, false),
		text:            lazygit.Style(t.DefaultFgColor, false),
	}
}

// Fixed colors lazygit uses regardless of theme, from its presentation code.
var (
	green   = lipgloss.NewStyle().Foreground(lipgloss.Color("2"))
	yellow  = lipgloss.NewStyle().Foreground(lipgloss.Color("3"))
	red     = lipgloss.NewStyle().Foreground(lipgloss.Color("1"))
	magenta = lipgloss.NewStyle().Foreground(lipgloss.Color("5"))
	cyan    = lipgloss.NewStyle().Foreground(lipgloss.Color("6"))
	dim     = lipgloss.NewStyle().Faint(true)
	bold    = lipgloss.NewStyle().Bold(true)
)

// spinner is lazygit's default loader.
var spinner = []string{"●∙∙", "∙●∙", "∙∙●", "∙●∙"}
