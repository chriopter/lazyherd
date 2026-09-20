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

	commitPrompt = "Write a git commit message for the change below.\n" +
		"Subject line: a changelog entry in past tense, under 72 characters, " +
		"starting with a verb such as Added, Fixed, Removed, Changed, Renamed, Moved, " +
		"naming the concrete thing that changed, e.g. 'Added description field to commit dialog' " +
		"or 'Fixed crash when scanning empty directories'. " +
		"No quotes, no trailing period, no prefixes like 'feat:' or 'chore:', " +
		"no vague words like 'update', 'improve', 'refactor', 'various'.\n" +
		"Then an empty line, then 1 to 5 dash bullets in the same style, " +
		"one concrete change each, wrapped at 72 characters. " +
		"Skip the bullets when the subject already says everything.\n" +
		"Follow any commit message conventions of this repository. " +
		"Output only the commit message, nothing else."
)

type generatedMsg struct {
	name    string
	subject string
	body    string
	err     error
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
		subject, body := splitMessage(string(out))
		if subject == "" {
			return generatedMsg{name: name, err: errors.New("claude returned nothing")}
		}
		return generatedMsg{name: name, subject: subject, body: body}
	}
}

// splitMessage separates a commit message into subject and body and strips
// quoting or code fences a model may wrap it in.
func splitMessage(text string) (subject, body string) {
	text = strings.TrimSpace(text)
	text = strings.TrimPrefix(text, "```")
	text = strings.TrimSuffix(text, "```")
	text = strings.TrimSpace(text)
	subject, body, _ = strings.Cut(text, "\n")
	subject = strings.Trim(strings.TrimSpace(subject), "\"'`")
	return subject, strings.TrimSpace(body)
}
