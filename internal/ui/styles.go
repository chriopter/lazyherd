package ui

import (
	"github.com/charmbracelet/lipgloss"
)

// theme is lazyherd's look, derived from the user's lazygit configuration so
// both panes match. Colors are the terminal's ANSI palette unless the user
// configured others.
type theme struct {
	frame           [6]rune
	icons           *icons
	activeBorder    lipgloss.Style
	inactiveBorder  lipgloss.Style
	searchingBorder lipgloss.Style
	options         lipgloss.Style
	selectedBg      lipgloss.Style
	unstaged        lipgloss.Style
	text            lipgloss.Style
}

func newTheme(cfg lazygitConfig) theme {
	t := cfg.Gui.Theme
	return theme{
		frame:           frameRunes(cfg.Gui.Border),
		icons:           configIcons(cfg.Gui.NerdFontsVersion),
		activeBorder:    configStyle(t.ActiveBorderColor, false),
		inactiveBorder:  configStyle(t.InactiveBorderColor, false),
		searchingBorder: configStyle(t.SearchingActiveBorderColor, false),
		options:         configStyle(t.OptionsTextColor, false),
		selectedBg:      configStyle(t.SelectedLineBgColor, true),
		unstaged:        configStyle(t.UnstagedChangesColor, false),
		text:            configStyle(t.DefaultFgColor, false),
	}
}

// Fixed colors lazygit uses regardless of theme, from its presentation code.
var (
	green  = lipgloss.NewStyle().Foreground(lipgloss.Color("2"))
	yellow = lipgloss.NewStyle().Foreground(lipgloss.Color("3"))
	red    = lipgloss.NewStyle().Foreground(lipgloss.Color("1"))
	cyan   = lipgloss.NewStyle().Foreground(lipgloss.Color("6"))
	dim    = lipgloss.NewStyle().Faint(true)
)

// spinner is lazygit's default loader.
var spinner = []string{"●∙∙", "∙●∙", "∙∙●", "∙●∙"}
