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

	panes  map[string][]Pane // repo name -> panes inside it
	labels map[string]string // workspace id -> label
}

// run executes a herdr command with a deadline.
func run(args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "herdr", args...)
	cmd.WaitDelay = time.Second
	return cmd.Output()
}

// do executes a herdr command and reports only success or failure.
func do(args ...string) error {
	_, err := run(args...)
	return err
}

func call(args ...string) (map[string]any, error) {
	out, err := run(args...)
	if err != nil {
		return nil, err
	}
	var v struct {
		Result map[string]any `json:"result"`
	}
	if err := json.Unmarshal(out, &v); err != nil {
		return nil, err
	}
	return v.Result, nil
}

func decode[T any](v any) (T, error) {
	var t T
	raw, err := json.Marshal(v)
	if err != nil {
		return t, err
	}
	return t, json.Unmarshal(raw, &t)
}

// Load maps every Herdr pane to the repository under root containing its cwd.
// A missing or stopped Herdr yields an empty, unavailable state.
func Load(root string) State {
	st := State{
		Workspace: os.Getenv("HERDR_WORKSPACE_ID"),
		panes:     map[string][]Pane{},
		labels:    map[string]string{},
	}
	res, err := call("pane", "list")
	if err != nil {
		return st
	}
	panes, err := decode[[]Pane](res["panes"])
	if err != nil {
		return st
	}
	st.Available = true
	for _, p := range panes {
		rel, err := filepath.Rel(root, p.Cwd)
		if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, "../") {
			continue
		}
		name := strings.SplitN(rel, string(filepath.Separator), 2)[0]
		st.panes[name] = append(st.panes[name], p)
	}
	if ws, err := call("workspace", "list"); err == nil {
		type workspace struct {
			ID    string `json:"workspace_id"`
			Label string `json:"label"`
		}
		if list, err := decode[[]workspace](ws["workspaces"]); err == nil {
			for _, w := range list {
				st.labels[w.ID] = w.Label
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

// HasRepos reports whether any pane of the workspace sits inside a repo.
func (s State) HasRepos(workspace string) bool {
	for _, ps := range s.panes {
		for _, p := range ps {
			if workspace == "" || p.WorkspaceID == workspace {
				return true
			}
		}
	}
	return false
}

// WorkspaceLabel is the display name of the current workspace.
func (s State) WorkspaceLabel() string {
	if l := s.labels[s.Workspace]; l != "" {
		return l
	}
	return s.Workspace
}

// SplitRight opens a shell pane to the right of pane without moving focus.
// ratio is the share of the width the original pane keeps. It returns the
// new pane's id.
func SplitRight(pane, cwd string, ratio float64) (string, error) {
	res, err := call("pane", "split", "--pane", pane, "--direction", "right",
		"--ratio", strconv.FormatFloat(ratio, 'f', 2, 64), "--cwd", cwd, "--no-focus")
	if err != nil {
		return "", err
	}
	p, err := decode[Pane](res["pane"])
	if err != nil || p.ID == "" {
		return "", errors.New("herdr: split returned no pane id")
	}
	return p.ID, nil
}

// Run submits a command line in a pane that sits at a shell prompt.
func Run(pane, command string) error {
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
