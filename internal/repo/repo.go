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
	st, err := git(gitTimeout, dir, "status", "--porcelain")
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

// Preview returns the colored short status and recent log of one repository.
// A failing git command is reported in place of its output.
func Preview(dir string, commits int) (status, log string) {
	status, err := git(gitTimeout, dir, "-c", "color.status=always", "status", "-sb")
	if err != nil {
		status = "git status failed: " + err.Error()
	}
	log, err = git(gitTimeout, dir, "log", "--color=always", "--date=relative", fmt.Sprintf("-%d", commits),
		"--pretty=format:%C(yellow)%h%C(reset) %C(dim)%ad%C(reset) %s")
	if err != nil {
		log = "git log failed: " + err.Error()
	}
	return status, log
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
