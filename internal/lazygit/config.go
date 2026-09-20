// Package lazygit reads the parts of the user's lazygit configuration that
// lazyherd mirrors: the theme, the border style and the icon font.
package lazygit

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"gopkg.in/yaml.v3"
)

// Config is the subset of lazygit's config.yml that affects lazyherd's look.
type Config struct {
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

// Default mirrors lazygit's built-in defaults.
func Default() Config {
	var c Config
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

// Path is where lazygit keeps its config on this platform.
func Path() string {
	if p := os.Getenv("LG_CONFIG_FILE"); p != "" {
		return strings.Split(p, ",")[0]
	}
	if p := os.Getenv("CONFIG_DIR"); p != "" {
		return filepath.Join(p, "config.yml")
	}
	base, err := os.UserConfigDir()
	if err != nil {
		return ""
	}
	if runtime.GOOS == "darwin" {
		base = filepath.Join(os.Getenv("HOME"), "Library", "Application Support")
	}
	return filepath.Join(base, "lazygit", "config.yml")
}

// Load reads the user's lazygit config on top of the defaults. Missing or
// broken files leave the defaults in place.
func Load(path string) Config {
	c := Default()
	raw, err := os.ReadFile(path)
	if err != nil {
		return c
	}
	var user Config
	if yaml.Unmarshal(raw, &user) != nil {
		return c
	}
	if user.Gui.Border != "" {
		c.Gui.Border = user.Gui.Border
	}
	c.Gui.NerdFontsVersion = user.Gui.NerdFontsVersion
	t, u := &c.Gui.Theme, &user.Gui.Theme
	for _, p := range []struct {
		dst *[]string
		src []string
	}{
		{&t.ActiveBorderColor, u.ActiveBorderColor},
		{&t.InactiveBorderColor, u.InactiveBorderColor},
		{&t.SearchingActiveBorderColor, u.SearchingActiveBorderColor},
		{&t.OptionsTextColor, u.OptionsTextColor},
		{&t.SelectedLineBgColor, u.SelectedLineBgColor},
		{&t.UnstagedChangesColor, u.UnstagedChangesColor},
		{&t.DefaultFgColor, u.DefaultFgColor},
	} {
		if len(p.src) > 0 {
			*p.dst = p.src
		}
	}
	return c
}

var ansi = map[string]string{
	"black": "0", "red": "1", "green": "2", "yellow": "3",
	"blue": "4", "magenta": "5", "cyan": "6", "white": "7",
}

// Style turns a lazygit color spec such as ["green", "bold"] into a style.
// With bg the color is applied as background, as for selectedLineBgColor.
func Style(spec []string, bg bool) lipgloss.Style {
	s := lipgloss.NewStyle()
	for _, word := range spec {
		switch word {
		case "bold":
			s = s.Bold(true)
		case "underline":
			s = s.Underline(true)
		case "reverse":
			s = s.Reverse(true)
		case "strikethrough":
			s = s.Strikethrough(true)
		case "default", "":
		default:
			color := word
			if c, ok := ansi[word]; ok {
				color = c
			}
			if bg {
				s = s.Background(lipgloss.Color(color))
			} else {
				s = s.Foreground(lipgloss.Color(color))
			}
		}
	}
	return s
}

// Icons are the nerd-font glyphs lazygit uses, per font version.
type Icons struct {
	Branch, DetachedHead string
}

// IconsFor returns the glyphs for a nerdFontsVersion, or nil when icons are off.
func IconsFor(version string) *Icons {
	switch version {
	case "3":
		return &Icons{Branch: "\U000f062c", DetachedHead: ""}
	case "2":
		return &Icons{Branch: "שׂ", DetachedHead: ""}
	}
	return nil
}

// FrameRunes returns the box characters for lazygit's border setting:
// horizontal, vertical, top-left, top-right, bottom-left, bottom-right.
func FrameRunes(border string) [6]rune {
	switch border {
	case "single", "double", "hidden", "bold":
		if border == "double" {
			return [6]rune{'═', '║', '╔', '╗', '╚', '╝'}
		}
		if border == "bold" {
			return [6]rune{'━', '┃', '┏', '┓', '┗', '┛'}
		}
		if border == "hidden" {
			return [6]rune{' ', ' ', ' ', ' ', ' ', ' '}
		}
		return [6]rune{'─', '│', '┌', '┐', '└', '┘'}
	}
	return [6]rune{'─', '│', '╭', '╮', '╰', '╯'}
}
