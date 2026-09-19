package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// pane is a Herdr pane whose working directory lies inside one of the repos.
type pane struct {
	PaneID      string `json:"pane_id"`
	TabID       string `json:"tab_id"`
	WorkspaceID string `json:"workspace_id"`
	Cwd         string `json:"cwd"`
}

// herdrState is what lazyherd knows about the running Herdr server.
type herdrState struct {
	available bool
	panes     map[string][]pane // repo name -> panes inside it
	wsLabels  map[string]string // workspace id -> label
	curWS     string            // HERDR_WORKSPACE_ID when running inside a Herdr pane
}

func herdrJSON(args ...string) (map[string]any, error) {
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

// loadHerdr maps every Herdr pane to the repository under root containing its cwd.
// A missing or stopped Herdr yields an empty, unavailable state.
func loadHerdr(root string) herdrState {
	st := herdrState{
		panes:    map[string][]pane{},
		wsLabels: map[string]string{},
		curWS:    os.Getenv("HERDR_WORKSPACE_ID"),
	}
	res, err := herdrJSON("pane", "list")
	if err != nil {
		return st
	}
	st.available = true
	for _, p := range decode[[]pane](res["panes"]) {
		rel, err := filepath.Rel(root, p.Cwd)
		if err != nil || rel == "." || strings.HasPrefix(rel, "..") {
			continue
		}
		name := strings.SplitN(rel, string(filepath.Separator), 2)[0]
		st.panes[name] = append(st.panes[name], p)
	}
	if ws, err := herdrJSON("workspace", "list"); err == nil {
		type workspace struct {
			ID    string `json:"workspace_id"`
			Label string `json:"label"`
		}
		for _, w := range decode[[]workspace](ws["workspaces"]) {
			st.wsLabels[w.ID] = w.Label
		}
	}
	return st
}

// pane returns the first pane of a repo, optionally restricted to a workspace.
func (h herdrState) pane(name, ws string) *pane {
	for i := range h.panes[name] {
		if ws == "" || h.panes[name][i].WorkspaceID == ws {
			return &h.panes[name][i]
		}
	}
	return nil
}

func (h herdrState) workspaceLabel() string {
	if l := h.wsLabels[h.curWS]; l != "" {
		return l
	}
	return h.curWS
}

func focusTab(tabID string) error { return exec.Command("herdr", "tab", "focus", tabID).Run() }

func createTab(ws, dir, label string) error {
	args := []string{"tab", "create", "--cwd", dir, "--label", label, "--focus"}
	if ws != "" {
		args = append(args, "--workspace", ws)
	}
	return exec.Command("herdr", args...).Run()
}
