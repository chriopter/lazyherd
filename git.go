package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	tea "github.com/charmbracelet/bubbletea"
)

// repo is the scanned state of one repository directly under the root.
type repo struct {
	name       string
	branch     string // current branch, or "@<hash>" when detached
	changes    int    // entries in git status --porcelain
	ahead      int
	behind     int
	noUpstream bool
}

func git(dir string, args ...string) (string, error) {
	out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).Output()
	return strings.TrimRight(string(out), "\n"), err
}

// scan reads every repository under root concurrently and returns them
// sorted: most changes first, then most out of sync, then by name.
func scan(root string) tea.Cmd {
	return func() tea.Msg {
		entries, _ := os.ReadDir(root)
		var (
			wg    sync.WaitGroup
			mu    sync.Mutex
			repos []repo
		)
		for _, e := range entries {
			dir := filepath.Join(root, e.Name())
			if !e.IsDir() {
				continue
			}
			if _, err := os.Stat(filepath.Join(dir, ".git")); err != nil {
				continue
			}
			wg.Add(1)
			go func(name, dir string) {
				defer wg.Done()
				r := scanRepo(name, dir)
				mu.Lock()
				repos = append(repos, r)
				mu.Unlock()
			}(e.Name(), dir)
		}
		wg.Wait()
		sort.Slice(repos, func(i, j int) bool {
			a, b := repos[i], repos[j]
			if a.changes != b.changes {
				return a.changes > b.changes
			}
			if a.ahead+a.behind != b.ahead+b.behind {
				return a.ahead+a.behind > b.ahead+b.behind
			}
			return a.name < b.name
		})
		return reposMsg(repos)
	}
}

func scanRepo(name, dir string) repo {
	r := repo{name: name}
	r.branch, _ = git(dir, "branch", "--show-current")
	if r.branch == "" {
		h, _ := git(dir, "rev-parse", "--short", "HEAD")
		r.branch = "@" + h
	}
	if st, _ := git(dir, "status", "--porcelain"); st != "" {
		r.changes = len(strings.Split(st, "\n"))
	}
	if ab, err := git(dir, "rev-list", "--left-right", "--count", "@{u}...HEAD"); err == nil {
		fmt.Sscanf(ab, "%d\t%d", &r.behind, &r.ahead)
	} else {
		r.noUpstream = true
	}
	return r
}

// fetchAll runs git fetch in every repository concurrently.
func fetchAll(root string, repos []repo) tea.Cmd {
	return func() tea.Msg {
		var wg sync.WaitGroup
		for _, r := range repos {
			wg.Add(1)
			go func(dir string) {
				defer wg.Done()
				git(dir, "fetch", "--quiet")
			}(filepath.Join(root, r.name))
		}
		wg.Wait()
		return fetchDoneMsg{}
	}
}

// loadPreview renders status and recent log of one repository, with colors.
func loadPreview(root, name string) tea.Cmd {
	return func() tea.Msg {
		dir := filepath.Join(root, name)
		st, _ := git(dir, "-c", "color.status=always", "status", "-sb")
		lg, _ := git(dir, "log", "--color=always", "--date=relative", "-14",
			"--pretty=format:%C(yellow)%h%C(reset) %C(dim)%ad%C(reset) %s")
		text := sSection.Render("STATUS") + "\n" + st + "\n" +
			sSection.Render("LOG") + "\n" + lg
		return previewMsg{name: name, text: text}
	}
}

func lazygit(dir string) *exec.Cmd { return exec.Command("lazygit", "-p", dir) }
