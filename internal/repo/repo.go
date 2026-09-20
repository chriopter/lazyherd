// Package repo scans a directory of Git repositories and runs Git in them.
package repo

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	maxParallel  = 8
	gitTimeout   = 15 * time.Second
	fetchTimeout = 90 * time.Second
)

// Repo is the scanned state of one repository directly under the root.
type Repo struct {
	Name       string
	Branch     string // current branch, or "@<hash>" when detached
	Changes    int    // entries in git status --porcelain
	Ahead      int
	Behind     int
	NoUpstream bool
	Err        error // git status failed; the counts above are unknown
}

// Dirty reports whether the working tree has uncommitted changes.
func (r Repo) Dirty() bool { return r.Changes > 0 }

func git(timeout time.Duration, dir string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	out, err := exec.CommandContext(ctx, "git", append([]string{"-C", dir}, args...)...).Output()
	return strings.TrimRight(string(out), "\n"), err
}

// parallel runs fn for every repo with at most maxParallel git processes at once.
func parallel(repos []Repo, fn func(i int, r Repo)) {
	var wg sync.WaitGroup
	sem := make(chan struct{}, maxParallel)
	for i, r := range repos {
		wg.Add(1)
		sem <- struct{}{}
		go func(i int, r Repo) {
			defer func() { <-sem; wg.Done() }()
			fn(i, r)
		}(i, r)
	}
	wg.Wait()
}

// Scan reads every repository directly under root concurrently and returns
// them sorted: most changes first, then most out of sync, then by name.
// Only immediate children are considered; symlinked directories are skipped.
func Scan(root string) ([]Repo, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}
	var repos []Repo
	for _, e := range entries {
		if e.IsDir() && isRepo(filepath.Join(root, e.Name())) {
			repos = append(repos, Repo{Name: e.Name()})
		}
	}
	parallel(repos, func(i int, r Repo) {
		repos[i] = scanOne(r.Name, filepath.Join(root, r.Name))
	})
	sort.Slice(repos, func(i, j int) bool {
		a, b := repos[i], repos[j]
		if a.Changes != b.Changes {
			return a.Changes > b.Changes
		}
		if a.Ahead+a.Behind != b.Ahead+b.Behind {
			return a.Ahead+a.Behind > b.Ahead+b.Behind
		}
		return a.Name < b.Name
	})
	return repos, nil
}

// isRepo accepts both a .git directory and the .git file of a worktree.
func isRepo(dir string) bool {
	_, err := os.Stat(filepath.Join(dir, ".git"))
	return err == nil
}

func scanOne(name, dir string) Repo {
	r := Repo{Name: name}
	st, err := git(gitTimeout, dir, "status", "--porcelain", "--untracked-files=all")
	if err != nil {
		r.Err = err
		return r
	}
	if st != "" {
		r.Changes = len(strings.Split(st, "\n"))
	}
	r.Branch, _ = git(gitTimeout, dir, "branch", "--show-current")
	if r.Branch == "" {
		h, _ := git(gitTimeout, dir, "rev-parse", "--short", "HEAD")
		r.Branch = "@" + h
	}
	if ab, err := git(gitTimeout, dir, "rev-list", "--left-right", "--count", "@{u}...HEAD"); err == nil {
		fmt.Sscanf(ab, "%d\t%d", &r.Behind, &r.Ahead)
	} else {
		r.NoUpstream = true
	}
	return r
}

// FetchAll runs git fetch --all in every repository and returns the names of
// the ones whose fetch failed. Repositories without remotes fetch nothing
// and succeed.
func FetchAll(root string, repos []Repo) []string {
	var (
		mu     sync.Mutex
		failed []string
	)
	parallel(repos, func(_ int, r Repo) {
		if _, err := git(fetchTimeout, filepath.Join(root, r.Name), "fetch", "--all", "--quiet"); err != nil {
			mu.Lock()
			failed = append(failed, r.Name)
			mu.Unlock()
		}
	})
	sort.Strings(failed)
	return failed
}

// Change is one entry of git status: a file with its index and worktree state.
type Change struct {
	Path      string
	Staged    byte // X column of git status --porcelain, ' ' when clean
	Unstaged  byte // Y column
	Untracked bool
}

// Code is the two-letter status as git shows it, e.g. "M ", " M", "??".
func (c Change) Code() string {
	if c.Untracked {
		return "??"
	}
	return string([]byte{c.Staged, c.Unstaged})
}

// Status returns the branch summary line and the changed files of dir.
func Status(dir string) (branch string, changes []Change, err error) {
	out, err := git(gitTimeout, dir, "status", "--porcelain=v1", "-b", "--untracked-files=all")
	if err != nil {
		return "", nil, err
	}
	for _, line := range strings.Split(out, "\n") {
		switch {
		case strings.HasPrefix(line, "## "):
			branch = strings.TrimPrefix(line, "## ")
		case len(line) > 3:
			c := Change{Staged: line[0], Unstaged: line[1], Path: line[3:]}
			if c.Staged == '?' {
				c.Untracked = true
			}
			if i := strings.Index(c.Path, " -> "); i >= 0 { // rename: keep the new name
				c.Path = c.Path[i+4:]
			}
			changes = append(changes, c)
		}
	}
	return branch, changes, nil
}

// Log returns the recent history of dir, colored.
func Log(dir string, commits int) string {
	out, err := git(gitTimeout, dir, "log", "--color=always", "--date=relative", fmt.Sprintf("-%d", commits),
		"--pretty=format:%C(yellow)%h%C(reset) %C(dim)%ad%C(reset) %s")
	if err != nil {
		return "git log failed: " + err.Error()
	}
	return out
}

// Diff returns the colored diff of one file: staged and unstaged hunks, or
// the whole file for an untracked one. The per-file header lines are dropped,
// the hunks start right away.
func Diff(dir string, c Change) string {
	if c.Untracked {
		// exit status 1 just means "differences found"
		out, _ := git(gitTimeout, dir, "diff", "--no-index", "--color=always", "--", os.DevNull, c.Path)
		return stripDiffHeader(out)
	}
	var parts []string
	if c.Staged != ' ' {
		if out, _ := git(gitTimeout, dir, "diff", "--cached", "--color=always", "--", c.Path); out != "" {
			parts = append(parts, stripDiffHeader(out))
		}
	}
	if c.Unstaged != ' ' {
		if out, _ := git(gitTimeout, dir, "diff", "--color=always", "--", c.Path); out != "" {
			parts = append(parts, stripDiffHeader(out))
		}
	}
	return strings.Join(parts, "\n")
}

// stripDiffHeader drops everything before the first hunk of a single-file
// diff, so the pane shows changes instead of "diff --git" boilerplate.
func stripDiffHeader(diff string) string {
	lines := strings.Split(diff, "\n")
	for i, l := range lines {
		if strings.Contains(l, "@@") {
			return strings.Join(lines[i:], "\n")
		}
	}
	return diff
}

// Run executes one git command in dir, for pull, push and single fetches.
func Run(dir string, args ...string) error {
	_, err := git(fetchTimeout, dir, args...)
	return err
}

// Commit stages everything in dir and commits it with the given message.
func Commit(dir, message string) error {
	if _, err := git(gitTimeout, dir, "add", "-A"); err != nil {
		return err
	}
	_, err := git(fetchTimeout, dir, "commit", "-q", "-m", message)
	return err
}

// Lazygit returns the command that opens lazygit in dir.
func Lazygit(dir string) *exec.Cmd { return exec.Command("lazygit", "-p", dir) }
