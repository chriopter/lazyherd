// lazyherd is a Herdr plugin: a cockpit over every Git repository in one
// directory, with a repo list that drives lazygit in the pane next to it.
package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"gopkg.in/yaml.v3"

	"github.com/chriopter/lazyherd/internal/herdr"
	"github.com/chriopter/lazyherd/internal/ui"
)

// version is set by goreleaser via -ldflags "-X main.version=...".
var version = "dev"

// Names Herdr knows the plugin and its cockpit pane by; they must match
// herdr-plugin.toml.
const (
	pluginID   = "chriopter.lazyherd"
	paneID     = "cockpit"
	paneTitle  = "lazyherd"
	configFile = "config.yml"
)

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "follow":
			// Companion mode, started by the cockpit in the pane next to it.
			if len(os.Args) != 3 {
				fail(errors.New("follow: expected a socket path"))
			}
			if err := herdr.Follow(os.Args[2]); err != nil {
				fail(fmt.Errorf("follow: %w", err))
			}
			return
		case "open":
			// The plugin action: focus the workspace's cockpit or open one.
			if err := herdr.OpenCockpit(pluginID, paneID, paneTitle, os.Getenv("HERDR_WORKSPACE_ID")); err != nil {
				fail(err)
			}
			return
		}
	}

	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), `Usage: lazyherd [flags] [DIR]
       lazyherd open

Lists every Git repository directly under DIR with its changes, branch and
sync state, and opens lazygit for the selected repo in a Herdr pane to the
right. DIR defaults to "root" in %s, then ~/git.

"open" is the plugin action: it focuses the current workspace's cockpit
pane or opens one.

Flags:
`, configPath())
		flag.PrintDefaults()
	}
	showVersion := flag.Bool("version", false, "print the version and exit")
	flag.BoolVar(showVersion, "v", false, "print the version and exit")
	flag.Parse()

	if *showVersion {
		fmt.Println("lazyherd " + version)
		return
	}
	if flag.NArg() > 1 {
		fmt.Fprintln(os.Stderr, "lazyherd: expected at most one directory")
		flag.Usage()
		os.Exit(2)
	}
	if os.Getenv("HERDR_PANE_ID") == "" || os.Getenv("HERDR_WORKSPACE_ID") == "" {
		fail(errors.New("runs inside a Herdr pane; install it with: herdr plugin install chriopter/lazyherd"))
	}

	root, err := rootDir(flag.Arg(0))
	if err != nil {
		fail(err)
	}
	if _, err := tea.NewProgram(ui.New(root, version), tea.WithAltScreen(), tea.WithMouseCellMotion()).Run(); err != nil {
		fail(err)
	}
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "lazyherd:", err)
	os.Exit(1)
}

// rootDir resolves the directory to scan: the argument, else the config
// file's root, else ~/git.
func rootDir(arg string) (string, error) {
	root := arg
	if root == "" {
		var config struct {
			Root string `yaml:"root"`
		}
		raw, err := os.ReadFile(configPath())
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return "", err
		}
		if err == nil {
			if err := yaml.Unmarshal(raw, &config); err != nil {
				return "", fmt.Errorf("%s: %w", configPath(), err)
			}
		}
		root = config.Root
	}
	home := os.Getenv("HOME")
	switch {
	case root == "":
		root = filepath.Join(home, "git")
	case root == "~" || strings.HasPrefix(root, "~/"):
		root = filepath.Join(home, root[1:])
	}
	if abs, err := filepath.Abs(root); err == nil {
		root = abs
	}
	if st, err := os.Stat(root); err != nil || !st.IsDir() {
		return "", fmt.Errorf("%s is not a directory", root)
	}
	return root, nil
}

// configPath is the plugin's config file: in the directory Herdr provides
// for it, or under ~/.config/lazyherd when run without the plugin.
func configPath() string {
	return filepath.Join(ui.ConfigDir(), configFile)
}
