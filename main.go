// lazyherd is a cockpit over every Git repository in one directory, with a
// jump into lazygit per repo and an optional link to Herdr workspaces.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/chriopter/lazyherd/internal/ui"
)

// version is set by goreleaser via -ldflags "-X main.version=...".
var version = "dev"

func main() {
	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), `Usage: lazyherd [flags] [DIR]

Shows every Git repository directly under DIR (default: ~/git) with its
changes, branch and sync state. Enter opens lazygit in the selected repo.

Flags:
`)
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

	root := filepath.Join(os.Getenv("HOME"), "git")
	if flag.NArg() == 1 {
		root = flag.Arg(0)
	}
	if abs, err := filepath.Abs(root); err == nil {
		root = abs
	}
	if st, err := os.Stat(root); err != nil || !st.IsDir() {
		fmt.Fprintf(os.Stderr, "lazyherd: %s is not a directory\n", root)
		os.Exit(1)
	}
	if _, err := tea.NewProgram(ui.New(root), tea.WithAltScreen()).Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
