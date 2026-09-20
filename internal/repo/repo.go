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
	Name string
	Status
	Err error // git status failed; the fields above are unknown
}

// Dirty reports whether the working tree has uncommitted changes.
func (r Repo) Dirty() bool { return len(r.Changes) > 0 }

func git(timeout time.Duration, dir string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", append([]string{"--no-optional-locks", "-C", dir}, args...)...)
	cmd.WaitDelay = time.Second
	out, err := cmd.Output()
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

// Scan reads every repository directly under root with one git call each and
// returns them sorted: most changes first, then most out of sync, then by
// name. Only immediate children are considered; symlinked directories are
// skipped.
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
		st, err := status(filepath.Join(root, r.Name))
		repos[i] = Repo{Name: r.Name, Status: st, Err: err}
	})
	sort.Slice(repos, func(i, j int) bool {
		a, b := repos[i], repos[j]
		if len(a.Changes) != len(b.Changes) {
			return len(a.Changes) > len(b.Changes)
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

// SyncResult is what Sync did in one repository.
type SyncResult struct {
	Name   string
	Pulled bool // new commits arrived
	Pushed bool
	Err    error
}

// Sync brings one repository in line with its upstream: a fast-forward pull,
// then a push when local commits are ahead. Repositories without an upstream
// are left alone.
func Sync(root string, r Repo) SyncResult {
	res := SyncResult{Name: r.Name}
	if r.NoUpstream || r.Err != nil {
		return res
	}
	dir := filepath.Join(root, r.Name)
	if _, err := git(fetchTimeout, dir, "pull", "--ff-only", "--quiet"); err != nil {
		res.Err = fmt.Errorf("pull: %w", err)
		return res
	}
	st, err := status(dir)
	if err != nil {
		res.Err = err
		return res
	}
	res.Pulled = st.Head != r.Head
	if st.Ahead > 0 {
		if _, err := git(fetchTimeout, dir, "push", "--quiet"); err != nil {
			res.Err = fmt.Errorf("push: %w", err)
			return res
		}
		res.Pushed = true
	}
	return res
}

// SyncAll runs Sync in every repository concurrently.
func SyncAll(root string, repos []Repo) []SyncResult {
	results := make([]SyncResult, len(repos))
	parallel(repos, func(i int, r Repo) { results[i] = Sync(root, r) })
	return results
}
