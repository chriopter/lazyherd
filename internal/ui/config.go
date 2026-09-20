package ui

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"gopkg.in/yaml.v3"
)

type lazygitConfig struct {
	Gui struct {
		Border           string `yaml:"border"`
		NerdFontsVersion string `yaml:"nerdFontsVersion"`
		Theme            struct {
			ActiveBorderColor          []string `yaml:"activeBorderColor"`
			InactiveBorderColor        []string `yaml:"inactiveBorderColor"`
			SearchingActiveBorderColor []string `yaml:"searchingActiveBorderColor"`
			OptionsTextColor           []string `yaml:"optionsTextColor"`
			SelectedLineBgColor        []string `yaml:"selectedLineBgColor"`
			UnstagedChangesColor       []string `yaml:"unstagedChangesColor"`
			DefaultFgColor             []string `yaml:"defaultFgColor"`
		} `yaml:"theme"`
	} `yaml:"gui"`
}

func defaultConfig() lazygitConfig {
	var c lazygitConfig
	c.Gui.Border = "rounded"
	c.Gui.Theme.ActiveBorderColor = []string{"green", "bold"}
	c.Gui.Theme.InactiveBorderColor = []string{"default"}
	c.Gui.Theme.SearchingActiveBorderColor = []string{"cyan", "bold"}
	c.Gui.Theme.OptionsTextColor = []string{"blue"}
	c.Gui.Theme.SelectedLineBgColor = []string{"blue"}
	c.Gui.Theme.UnstagedChangesColor = []string{"red"}
	c.Gui.Theme.DefaultFgColor = []string{"default"}
	return c
}

func lazygitConfigPath() string {
	if path := os.Getenv("LG_CONFIG_FILE"); path != "" {
		return strings.Split(path, ",")[0]
	}
	if dir := os.Getenv("CONFIG_DIR"); dir != "" {
		return filepath.Join(dir, "config.yml")
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		return ""
	}
	if runtime.GOOS == "darwin" {
		dir = filepath.Join(os.Getenv("HOME"), "Library", "Application Support")
	}
	return filepath.Join(dir, "lazygit", "config.yml")
}

func loadConfig(path string) lazygitConfig {
	c := defaultConfig()
	raw, err := os.ReadFile(path)
	if err != nil {
		return c
	}
	var user lazygitConfig
	if yaml.Unmarshal(raw, &user) != nil {
		return c
	}
	if user.Gui.Border != "" {
		c.Gui.Border = user.Gui.Border
	}
	c.Gui.NerdFontsVersion = user.Gui.NerdFontsVersion
	theme, overrides := &c.Gui.Theme, &user.Gui.Theme
	for _, color := range []struct {
		value    *[]string
		override []string
	}{
		{&theme.ActiveBorderColor, overrides.ActiveBorderColor},
		{&theme.InactiveBorderColor, overrides.InactiveBorderColor},
		{&theme.SearchingActiveBorderColor, overrides.SearchingActiveBorderColor},
		{&theme.OptionsTextColor, overrides.OptionsTextColor},
		{&theme.SelectedLineBgColor, overrides.SelectedLineBgColor},
		{&theme.UnstagedChangesColor, overrides.UnstagedChangesColor},
		{&theme.DefaultFgColor, overrides.DefaultFgColor},
	} {
		if len(color.override) > 0 {
			*color.value = color.override
		}
	}
	return c
}

var ansiColor = map[string]string{
	"black": "0", "red": "1", "green": "2", "yellow": "3",
	"blue": "4", "magenta": "5", "cyan": "6", "white": "7",
}

func configStyle(spec []string, background bool) lipgloss.Style {
	style := lipgloss.NewStyle()
	for _, word := range spec {
		switch word {
		case "bold":
			style = style.Bold(true)
		case "underline":
			style = style.Underline(true)
		case "reverse":
			style = style.Reverse(true)
		case "strikethrough":
			style = style.Strikethrough(true)
		case "default", "":
		default:
			color := word
			if ansi, ok := ansiColor[word]; ok {
				color = ansi
			}
			if background {
				style = style.Background(lipgloss.Color(color))
			} else {
				style = style.Foreground(lipgloss.Color(color))
			}
		}
	}
	return style
}

type icons struct {
	branch, detachedHead string
}

func configIcons(version string) *icons {
	switch version {
	case "3":
		return &icons{branch: "\U000f062c", detachedHead: ""}
	case "2":
		return &icons{branch: "שׂ", detachedHead: ""}
	default:
		return nil
	}
}

func frameRunes(border string) [6]rune {
	switch border {
	case "double":
		return [6]rune{'═', '║', '╔', '╗', '╚', '╝'}
	case "bold":
		return [6]rune{'━', '┃', '┏', '┓', '┗', '┛'}
	case "hidden":
		return [6]rune{' ', ' ', ' ', ' ', ' ', ' '}
	case "single":
		return [6]rune{'─', '│', '┌', '┐', '└', '┘'}
	default:
		return [6]rune{'─', '│', '╭', '╮', '╰', '╯'}
	}
}
