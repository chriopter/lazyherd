// Package herdr talks to a running Herdr server through its CLI.
package herdr

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const timeout = 5 * time.Second

// Pane is a Herdr pane whose working directory lies inside one of the repos.
type Pane struct {
	ID          string `json:"pane_id"`
	TabID       string `json:"tab_id"`
	WorkspaceID string `json:"workspace_id"`
	Cwd         string `json:"cwd"`
}

// State is what lazyherd knows about the running Herdr server.
type State struct {
	Available bool
	Workspace string // HERDR_WORKSPACE_ID when running inside a Herdr pane

	panes          map[string][]Pane // repo name -> panes inside it
	workspaceLabel string
}

func run(args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "herdr", args...)
	cmd.WaitDelay = time.Second
	return cmd.Output()
}

func do(args ...string) error {
	_, err := run(args...)
	return err
}

func call(result any, args ...string) error {
	out, err := run(args...)
	if err != nil {
		return err
	}
	var v struct {
		Result json.RawMessage `json:"result"`
	}
	if err := json.Unmarshal(out, &v); err != nil {
		return err
	}
	return json.Unmarshal(v.Result, result)
}

// Load maps every Herdr pane to the repository under root containing its cwd.
// A missing or stopped Herdr yields an empty, unavailable state.
func Load(root string) State {
	st := State{
		Workspace: os.Getenv("HERDR_WORKSPACE_ID"),
		panes:     map[string][]Pane{},
	}
	var paneList struct {
		Panes []Pane `json:"panes"`
	}
	if err := call(&paneList, "pane", "list"); err != nil {
		return st
	}
	st.Available = true
	for _, p := range paneList.Panes {
		rel, err := filepath.Rel(root, p.Cwd)
		if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, "../") {
			continue
		}
		name := strings.SplitN(rel, string(filepath.Separator), 2)[0]
		st.panes[name] = append(st.panes[name], p)
	}
	var workspaceList struct {
		Workspaces []struct {
			ID    string `json:"workspace_id"`
			Label string `json:"label"`
		} `json:"workspaces"`
	}
	if err := call(&workspaceList, "workspace", "list"); err == nil {
		for _, w := range workspaceList.Workspaces {
			if w.ID == st.Workspace {
				st.workspaceLabel = w.Label
				break
			}
		}
	}
	return st
}

// Pane returns the first pane inside a repo, optionally restricted to a workspace.
func (s State) Pane(repo, workspace string) *Pane {
	for i := range s.panes[repo] {
		if workspace == "" || s.panes[repo][i].WorkspaceID == workspace {
			return &s.panes[repo][i]
		}
	}
	return nil
}

// WorkspaceLabel is the display name of the current workspace.
func (s State) WorkspaceLabel() string {
	if s.workspaceLabel != "" {
		return s.workspaceLabel
	}
	return s.Workspace
}

func splitRight(pane, cwd string, ratio float64) (string, error) {
	var result struct {
		Pane Pane `json:"pane"`
	}
	if err := call(&result, "pane", "split", "--pane", pane, "--direction", "right",
		"--ratio", strconv.FormatFloat(ratio, 'f', 2, 64), "--cwd", cwd, "--no-focus"); err != nil {
		return "", err
	}
	if result.Pane.ID == "" {
		return "", errors.New("herdr: split returned no pane id")
	}
	return result.Pane.ID, nil
}

func runInPane(pane, command string) error {
	return do("pane", "run", pane, command)
}

// ClosePane closes a pane.
func ClosePane(pane string) error { return do("pane", "close", pane) }

// FocusRight moves focus to the pane right of pane.
func FocusRight(pane string) error {
	return do("pane", "focus", "--direction", "right", "--pane", pane)
}

// FocusTab brings a tab to the front.
func FocusTab(tabID string) error { return do("tab", "focus", tabID) }

// CreateTab opens and focuses a new tab in dir, in the given workspace if set.
func CreateTab(workspace, dir, label string) error {
	args := []string{"tab", "create", "--cwd", dir, "--label", label, "--focus"}
	if workspace != "" {
		args = append(args, "--workspace", workspace)
	}
	return do(args...)
}
