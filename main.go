// lazyherd is a cockpit over every Git repository in one directory, with a
// jump into lazygit per repo and an optional link to Herdr workspaces.
package main

import (
	"fmt"
	"os"
	"path/filepath"

	tea "github.com/charmbracelet/bubbletea"
)

// version is set by goreleaser via -ldflags "-X main.version=...".
var version = "dev"

func usage() {
	fmt.Fprintf(os.Stderr, `lazyherd %s

Usage: lazyherd [DIR]

Shows every Git repository directly under DIR (default: ~/git) with its
changes, branch and sync state. Enter opens lazygit in the selected repo.

Options:
  -h, --help     Show this help
  -v, --version  Print the version
`, version)
}

func main() {
	root := filepath.Join(os.Getenv("HOME"), "git")
	for _, a := range os.Args[1:] {
		switch a {
		case "-h", "--help":
			usage()
			return
		case "-v", "--version":
			fmt.Println("lazyherd " + version)
			return
		default:
			root = a
		}
	}
	if abs, err := filepath.Abs(root); err == nil {
		root = abs
	}
	if st, err := os.Stat(root); err != nil || !st.IsDir() {
		fmt.Fprintf(os.Stderr, "lazyherd: %s is not a directory\n", root)
		os.Exit(1)
	}
	if _, err := tea.NewProgram(newModel(root), tea.WithAltScreen()).Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
