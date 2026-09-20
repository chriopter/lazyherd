package ui

import (
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"

	"github.com/charmbracelet/lipgloss"
)

func TestConfigStyleColorsAndAttributes(t *testing.T) {
	for name, value := range map[string]string{"black": "0", "red": "1", "green": "2", "yellow": "3", "blue": "4", "magenta": "5", "cyan": "6", "white": "7", "#1aB2c3": "#1aB2c3"} {
		for _, background := range []bool{false, true} {
			t.Run(name+map[bool]string{false: "/foreground", true: "/background"}[background], func(t *testing.T) {
				s := configStyle([]string{name, "bold", "underline", "reverse", "strikethrough"}, background)
				color := s.GetForeground()
				if background {
					color = s.GetBackground()
				}
				if color != lipgloss.Color(value) || !s.GetBold() || !s.GetUnderline() || !s.GetReverse() || !s.GetStrikethrough() {
					t.Fatalf("style for %s: %+v", name, s)
				}
				other := s.GetBackground()
				if background {
					other = s.GetForeground()
				}
				if other != (lipgloss.NoColor{}) {
					t.Fatalf("unrequested color: %v", other)
				}
			})
		}
	}
	for _, spec := range [][]string{nil, {}, {"default"}, {"", "default"}} {
		s := configStyle(spec, false)
		if s.GetForeground() != (lipgloss.NoColor{}) || s.GetBackground() != (lipgloss.NoColor{}) || s.GetBold() || s.GetUnderline() {
			t.Fatalf("default set attributes: %+v", s)
		}
	}
	s := configStyle([]string{"bold", "red", "underline", "#abcdef"}, false)
	if s.GetForeground() != lipgloss.Color("#abcdef") || !s.GetBold() || !s.GetUnderline() {
		t.Fatal("last color or combined attributes lost")
	}
}

func TestLoadConfigMergeAllThemeFields(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yml")
	raw := `gui:
  border: double
  nerdFontsVersion: "2"
  theme:
    activeBorderColor: [red, underline]
    inactiveBorderColor: [yellow]
    searchingActiveBorderColor: [magenta, bold]
    optionsTextColor: ['#123456']
    selectedLineBgColor: [cyan]
    unstagedChangesColor: [green]
    defaultFgColor: [white]
`
	if err := os.WriteFile(path, []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}
	want := defaultConfig()
	want.Gui.Border = "double"
	want.Gui.NerdFontsVersion = "2"
	want.Gui.Theme.ActiveBorderColor = []string{"red", "underline"}
	want.Gui.Theme.InactiveBorderColor = []string{"yellow"}
	want.Gui.Theme.SearchingActiveBorderColor = []string{"magenta", "bold"}
	want.Gui.Theme.OptionsTextColor = []string{"#123456"}
	want.Gui.Theme.SelectedLineBgColor = []string{"cyan"}
	want.Gui.Theme.UnstagedChangesColor = []string{"green"}
	want.Gui.Theme.DefaultFgColor = []string{"white"}
	got := loadConfig(path)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
	theme := newTheme(got)
	if theme.frame != frameRunes("double") || theme.icons == nil || theme.icons.branch != "שׂ" || theme.activeBorder.GetForeground() != lipgloss.Color("1") || !theme.activeBorder.GetUnderline() || theme.selectedBg.GetBackground() != lipgloss.Color("6") || theme.options.GetForeground() != lipgloss.Color("#123456") {
		t.Fatalf("theme not wired to config: %+v", theme)
	}
}

func TestLoadConfigEmptyAndInvalidKeepDefaults(t *testing.T) {
	for _, raw := range []string{"", "{}", "gui: {}", "gui:\n  border: ''\n  theme:\n    activeBorderColor: []\n    optionsTextColor: null\n", "gui: [invalid", "gui:\n  theme:\n    activeBorderColor: not-a-list\n"} {
		t.Run(raw, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.yml")
			if err := os.WriteFile(path, []byte(raw), 0o644); err != nil {
				t.Fatal(err)
			}
			if got := loadConfig(path); !reflect.DeepEqual(got, defaultConfig()) {
				t.Fatalf("got %+v", got)
			}
		})
	}
}

func TestLazygitConfigPathPrecedence(t *testing.T) {
	root := t.TempDir()
	t.Setenv("HOME", root)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(root, "xdg"))
	t.Setenv("CONFIG_DIR", filepath.Join(root, "override"))
	t.Setenv("LG_CONFIG_FILE", "first.yml,second.yml")
	if got := lazygitConfigPath(); got != "first.yml" {
		t.Fatalf("LG_CONFIG_FILE: %q", got)
	}
	t.Setenv("LG_CONFIG_FILE", "")
	if got := lazygitConfigPath(); got != filepath.Join(root, "override", "config.yml") {
		t.Fatalf("CONFIG_DIR: %q", got)
	}
	t.Setenv("CONFIG_DIR", "")
	want := filepath.Join(root, "xdg", "lazygit", "config.yml")
	if runtime.GOOS == "darwin" {
		want = filepath.Join(root, "Library", "Application Support", "lazygit", "config.yml")
	}
	if got := lazygitConfigPath(); got != want {
		t.Fatalf("user config path: %q, want %q", got, want)
	}
	t.Setenv("XDG_CONFIG_HOME", "")
	if runtime.GOOS != "darwin" {
		want = filepath.Join(root, ".config", "lazygit", "config.yml")
	}
	if got := lazygitConfigPath(); got != want {
		t.Fatalf("home config path: %q, want %q", got, want)
	}
}

func TestNewLoadsSelectedConfigAndPinsPath(t *testing.T) {
	root := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", root)
	t.Setenv("HOME", root)
	t.Setenv("HERDR_PANE_ID", "own")
	config := filepath.Join(root, "explicit.yml")
	if err := os.WriteFile(config, []byte("gui:\n  border: bold\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("LG_CONFIG_FILE", config)
	t.Setenv("CONFIG_DIR", filepath.Join(root, "missing"))
	m := New(root, "test-version")
	if m.theme.frame != frameRunes("bold") || m.version != "test-version" || m.ownPane != "own" || m.activity != "scanning" {
		t.Fatalf("New: %+v", m)
	}
	base, err := os.UserConfigDir()
	if err != nil {
		t.Fatal(err)
	}
	if m.pins.path != filepath.Join(base, "lazyherd", "pins.json") {
		t.Fatalf("pin path: %q", m.pins.path)
	}
}

func TestIconsAndBorderFallback(t *testing.T) {
	for _, version := range []string{"", "1", "4", "unknown"} {
		if configIcons(version) != nil {
			t.Errorf("icons enabled for %q", version)
		}
	}
	for _, version := range []string{"2", "3"} {
		icons := configIcons(version)
		if icons == nil || icons.branch == "" || icons.detachedHead == "" {
			t.Fatalf("missing icons for %q", version)
		}
	}
	if frameRunes("unknown") != frameRunes("rounded") {
		t.Fatal("unknown border must fall back to rounded")
	}
}
