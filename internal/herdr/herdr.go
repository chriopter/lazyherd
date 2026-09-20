// Package herdr talks to a running Herdr server through its CLI.
package herdr

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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
	Label       string `json:"label"`
}

// State is what lazyherd knows about the running Herdr server.
type State struct {
	Workspace string // HERDR_WORKSPACE_ID of the pane lazyherd runs in

	panes          map[string][]Pane // repo name -> panes inside it
	workspaceLabel string
}

// binary is the herdr executable: the one that started us as a plugin, or
// whatever is on PATH.
func binary() string {
	if bin := os.Getenv("HERDR_BIN_PATH"); bin != "" {
		return bin
	}
	return "herdr"
}

// run executes a herdr subcommand; a failure carries the last line herdr
// printed to stderr so the status bar shows the reason, not "exit status 1".
func run(args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, binary(), args...)
	cmd.WaitDelay = time.Second
	out, err := cmd.Output()
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		if ctx.Err() != nil {
			return nil, fmt.Errorf("herdr %s: timed out", args[0]+" "+args[1])
		}
		if msg := lastLine(exit.Stderr); msg != "" {
			return nil, fmt.Errorf("herdr %s: %s", args[0]+" "+args[1], msg)
		}
		return nil, fmt.Errorf("herdr %s: %w", args[0]+" "+args[1], err)
	}
	return out, err
}

// lastLine returns the last stderr line, unwrapping herdr's JSON error
// envelope ({"error":{"message":…}}) to its message when that is what it is.
func lastLine(b []byte) string {
	lines := strings.Split(strings.TrimSpace(string(b)), "\n")
	line := strings.TrimSpace(lines[len(lines)-1])
	var envelope struct {
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if json.Unmarshal([]byte(line), &envelope) == nil && envelope.Error.Message != "" {
		return envelope.Error.Message
	}
	return line
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
// When Herdr cannot be asked, the state is empty and the error says why.
func Load(root string) (State, error) {
	st := State{
		Workspace: os.Getenv("HERDR_WORKSPACE_ID"),
		panes:     map[string][]Pane{},
	}
	panes, err := listPanes()
	if err != nil {
		return st, err
	}
	for _, p := range panes {
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
	return st, nil
}

func listPanes() ([]Pane, error) {
	var list struct {
		Panes []Pane `json:"panes"`
	}
	if err := call(&list, "pane", "list"); err != nil {
		return nil, err
	}
	return list.Panes, nil
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

// OpenCockpit brings the workspace's cockpit pane to the front, opening the
// plugin's pane entrypoint when there is none yet. Herdr labels a plugin pane
// with its manifest title, which is how an existing one is recognised.
func OpenCockpit(plugin, entrypoint, label, workspace string) error {
	panes, err := listPanes()
	if err != nil {
		return err
	}
	for _, p := range panes {
		if p.WorkspaceID == workspace && p.Label == label {
			return do("plugin", "pane", "focus", p.ID)
		}
	}
	return do("plugin", "pane", "open", "--plugin", plugin, "--entrypoint", entrypoint, "--workspace", workspace, "--focus")
}
