// Package pins remembers which repositories belong to a Herdr workspace by
// the user's choice, on top of the ones Herdr reports panes in.
package pins

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
)

// Store maps a workspace name to the repositories pinned to it.
type Store struct {
	path string
	byWS map[string][]string
}

// Path is the default location of the pin file.
func Path() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		dir = filepath.Join(os.Getenv("HOME"), ".config")
	}
	return filepath.Join(dir, "lazyherd", "pins.json")
}

// Load reads the store; a missing file yields an empty store.
func Load(path string) (*Store, error) {
	s := &Store{path: path, byWS: map[string][]string{}}
	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return s, err
	}
	return s, json.Unmarshal(raw, &s.byWS)
}

// Pinned reports whether repo is pinned to workspace.
func (s *Store) Pinned(workspace, repo string) bool {
	for _, r := range s.byWS[workspace] {
		if r == repo {
			return true
		}
	}
	return false
}

// Toggle pins or unpins repo for workspace and saves the store.
func (s *Store) Toggle(workspace, repo string) error {
	list := s.byWS[workspace]
	if s.Pinned(workspace, repo) {
		kept := list[:0]
		for _, r := range list {
			if r != repo {
				kept = append(kept, r)
			}
		}
		list = kept
	} else {
		list = append(list, repo)
		sort.Strings(list)
	}
	if len(list) == 0 {
		delete(s.byWS, workspace)
	} else {
		s.byWS[workspace] = list
	}
	return s.save()
}

func (s *Store) save() error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(s.byWS, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.path, append(raw, '\n'), 0o644)
}
