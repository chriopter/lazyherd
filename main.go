// lazyherd is a cockpit over every Git repository in one directory: a repo
// list that drives lazygit in a Herdr pane next to it.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/chriopter/lazyherd/internal/follow"
	"github.com/chriopter/lazyherd/internal/ui"
)

// version is set by goreleaser via -ldflags "-X main.version=...".
var version = "dev"

func main() {
	if len(os.Args) == 3 && os.Args[1] == "follow" {
		// Companion mode, started by lazyherd itself in a Herdr pane.
		if err := follow.Run(os.Args[2]); err != nil {
			fmt.Fprintln(os.Stderr, "lazyherd follow:", err)
			os.Exit(1)
		}
		return
	}

	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), `Usage: lazyherd [flags] [DIR]

Lists every Git repository directly under DIR (default: ~/git) with its
changes, branch and sync state. Inside a Herdr pane it opens lazygit for
the selected repo in a pane to the right; elsewhere Enter opens lazygit.

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
	if _, err := tea.NewProgram(ui.New(root), tea.WithAltScreen(), tea.WithMouseCellMotion()).Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
