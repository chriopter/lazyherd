package ui

import (
	"context"
	"errors"
	"os/exec"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/chriopter/lazyherd/internal/repo"
)

const (
	claudeTimeout = 2 * time.Minute
	diffLimit     = 60_000

	commitPrompt = "Write a git commit message for the change below. One short line, " +
		"under 72 characters, no quotes, no trailing period, no explanation. " +
		"Follow any commit message conventions of this repository. Output only the message."
)

type generatedMsg struct {
	name string
	text string
	err  error
}

// generateCmd asks Claude Code (claude -p) for a commit message. It runs in
// the repository so that the repository's CLAUDE.md conventions apply.
func generateCmd(root, name string) tea.Cmd {
	return func() tea.Msg {
		if _, err := exec.LookPath("claude"); err != nil {
			return generatedMsg{name: name, err: errors.New("claude not found on PATH")}
		}
		dir := root + "/" + name
		ctx, cancel := context.WithTimeout(context.Background(), claudeTimeout)
		defer cancel()
		cmd := exec.CommandContext(ctx, "claude", "-p", commitPrompt)
		cmd.Dir = dir
		cmd.Stdin = strings.NewReader(repo.DiffAll(dir, diffLimit))
		out, err := cmd.Output()
		if err != nil {
			return generatedMsg{name: name, err: err}
		}
		text := strings.TrimSpace(string(out))
		if i := strings.IndexByte(text, '\n'); i >= 0 {
			text = text[:i] // keep the first line only
		}
		text = strings.Trim(text, "\"'`")
		if text == "" {
			return generatedMsg{name: name, err: errors.New("claude returned nothing")}
		}
		return generatedMsg{name: name, text: text}
	}
}
