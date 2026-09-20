package ui

import "github.com/charmbracelet/lipgloss"

var (
	colorAccent   = lipgloss.AdaptiveColor{Light: "#5A4FCF", Dark: "#8B7CFF"}
	colorDim      = lipgloss.AdaptiveColor{Light: "#8A8A8A", Dark: "#6C6C6C"}
	colorText     = lipgloss.AdaptiveColor{Light: "#222222", Dark: "#DDDDDD"}
	colorWarn     = lipgloss.AdaptiveColor{Light: "#B26B00", Dark: "#F2B34E"}
	colorGood     = lipgloss.AdaptiveColor{Light: "#1F7A3A", Dark: "#6CD483"}
	colorBad      = lipgloss.AdaptiveColor{Light: "#B0303A", Dark: "#FF7B85"}
	colorSelected = lipgloss.AdaptiveColor{Light: "#E6E2FF", Dark: "#33305A"}

	titleStyle        = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#FFFFFF")).Background(colorAccent).Padding(0, 1)
	headerStyle       = lipgloss.NewStyle().Bold(true).Foreground(colorDim)
	textStyle         = lipgloss.NewStyle().Foreground(colorText)
	dimStyle          = lipgloss.NewStyle().Foreground(colorDim)
	dirtyStyle        = lipgloss.NewStyle().Foreground(colorWarn).Bold(true)
	cleanStyle        = lipgloss.NewStyle().Foreground(colorGood)
	aheadStyle        = lipgloss.NewStyle().Foreground(colorGood)
	behindStyle       = lipgloss.NewStyle().Foreground(colorBad)
	errorStyle        = lipgloss.NewStyle().Foreground(colorBad).Bold(true)
	branchStyle       = lipgloss.NewStyle().Foreground(colorAccent)
	keyStyle          = lipgloss.NewStyle().Bold(true).Foreground(colorAccent)
	filterStyle       = lipgloss.NewStyle().Foreground(colorWarn).Bold(true)
	paneStyle         = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(colorDim).Padding(0, 1)
	activePaneStyle   = paneStyle.BorderForeground(colorAccent)
	previewTitleStyle = lipgloss.NewStyle().Bold(true).Foreground(colorAccent)
	sectionStyle      = lipgloss.NewStyle().Bold(true).Foreground(colorDim)
)
