package ui

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
)

type pins struct {
	path        string
	byWorkspace map[string][]string
}

// ConfigDir holds the user's settings: the directory Herdr provides for the
// plugin, or ~/.config/lazyherd when the binary runs on its own.
func ConfigDir() string {
	if dir := os.Getenv("HERDR_PLUGIN_CONFIG_DIR"); dir != "" {
		return dir
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		dir = filepath.Join(os.Getenv("HOME"), ".config")
	}
	return filepath.Join(dir, "lazyherd")
}

// pinsPath is where pins live: Herdr's state directory for the plugin, or
// next to the config when the binary runs on its own.
func pinsPath() string {
	if dir := os.Getenv("HERDR_PLUGIN_STATE_DIR"); dir != "" {
		return filepath.Join(dir, "pins.json")
	}
	return filepath.Join(ConfigDir(), "pins.json")
}

func loadPins(path string) (*pins, error) {
	p := &pins{path: path, byWorkspace: map[string][]string{}}
	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return p, nil
	}
	if err != nil {
		return p, err
	}
	if err := json.Unmarshal(raw, &p.byWorkspace); err != nil {
		return p, fmt.Errorf("%s: %w", path, err)
	}
	if p.byWorkspace == nil {
		p.byWorkspace = map[string][]string{}
	}
	return p, nil
}

func (p *pins) has(workspace, repo string) bool {
	return slices.Contains(p.byWorkspace[workspace], repo)
}

func (p *pins) toggle(workspace, repo string) error {
	list := p.byWorkspace[workspace]
	if p.has(workspace, repo) {
		list = slices.DeleteFunc(list, func(name string) bool { return name == repo })
	} else {
		list = append(list, repo)
		slices.Sort(list)
	}
	if len(list) == 0 {
		delete(p.byWorkspace, workspace)
	} else {
		p.byWorkspace[workspace] = list
	}
	return p.save()
}

func (p *pins) save() error {
	if err := os.MkdirAll(filepath.Dir(p.path), 0o755); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(p.byWorkspace, "", "  ")
	if err != nil {
		return err
	}
	tmp := p.path + ".tmp"
	if err := os.WriteFile(tmp, append(raw, '\n'), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, p.path)
}
