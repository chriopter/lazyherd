// Package herdr talks to a running Herdr server through its CLI.
package herdr

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

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

func call(args ...string) (map[string]any, error) {
	out, err := exec.Command("herdr", args...).Output()
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

func decode[T any](v any) T {
	var t T
	raw, _ := json.Marshal(v)
	json.Unmarshal(raw, &t)
	return t
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
	st.Available = true
	for _, p := range decode[[]Pane](res["panes"]) {
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
		for _, w := range decode[[]workspace](ws["workspaces"]) {
			st.labels[w.ID] = w.Label
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
	if l := s.labels[s.Workspace]; l != "" {
		return l
	}
	return s.Workspace
}

// FocusTab brings a tab to the front.
func FocusTab(tabID string) error { return exec.Command("herdr", "tab", "focus", tabID).Run() }

// CreateTab opens and focuses a new tab in dir, in the given workspace if set.
func CreateTab(workspace, dir, label string) error {
	args := []string{"tab", "create", "--cwd", dir, "--label", label, "--focus"}
	if workspace != "" {
		args = append(args, "--workspace", workspace)
	}
	return exec.Command("herdr", args...).Run()
}
