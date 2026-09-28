package repo

import (
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// Commit is one entry of the timeline across repositories.
type Commit struct {
	Repo    string
	Hash    string
	Time    int64 // committer date, unix
	Author  string
	Subject string
}

// timelineFormat separates fields with the unit separator, which neither
// names nor subjects contain.
const timelineFormat = "--format=%H%x1f%ct%x1f%an%x1f%s"

// Timeline returns the newest commits of every repository, newest first:
// at most perRepo from each repository's local and remote-tracking branches,
// at most limit in total. Repositories without commits contribute nothing.
func Timeline(root string, repos []Repo, perRepo, limit int) []Commit {
	lists := make([][]Commit, len(repos))
	parallel(repos, func(i int, r Repo) {
		out, err := git(gitTimeout, filepath.Join(root, r.Name), "log", "--branches", "--remotes", "-n", strconv.Itoa(perRepo), timelineFormat)
		if err == nil {
			lists[i] = parseLog(r.Name, out)
		}
	})
	var all []Commit
	for _, l := range lists {
		all = append(all, l...)
	}
	sort.SliceStable(all, func(i, j int) bool {
		if all[i].Time != all[j].Time {
			return all[i].Time > all[j].Time
		}
		return all[i].Repo < all[j].Repo
	})
	if len(all) > limit {
		all = all[:limit]
	}
	return all
}

func parseLog(name, out string) []Commit {
	var commits []Commit
	for _, line := range strings.Split(out, "\n") {
		f := strings.SplitN(line, "\x1f", 4)
		if len(f) < 4 {
			continue
		}
		t, _ := strconv.ParseInt(f[1], 10, 64)
		commits = append(commits, Commit{Repo: name, Hash: f[0], Time: t, Author: f[2], Subject: f[3]})
	}
	return commits
}

// WorkTree stands for the uncommitted changes where a commit id is expected.
const WorkTree = "WORKTREE"

// Changes renders one change of the repository in dir for the pager, in
// git's colors: a commit with its changed files, line totals and patch, or
// for WorkTree the staged, unstaged and untracked files with their patches.
// WorkTree may carry a ":version" suffix, which is ignored. width fits the
// file statistics to the pane.
func Changes(dir, rev string, width int) (string, error) {
	stat := "--stat=" + strconv.Itoa(max(width, 40))
	if base, _, _ := strings.Cut(rev, ":"); base != WorkTree {
		return git(gitTimeout, dir, "show", "--color=always", stat, "--summary", "--patch",
			"--format=%C(yellow)%h%C(reset) %C(bold)%s%C(reset)%n%C(green)%an%C(reset) · %ad (%ar)%n%+b", "--date=format:%Y-%m-%d %H:%M", rev)
	}
	var b strings.Builder
	for _, s := range []struct {
		title string
		args  []string
	}{
		{"Staged", []string{"diff", "--cached", "--color=always", stat}},
		{"Unstaged", []string{"diff", "--color=always", stat}},
		{"Untracked", []string{"ls-files", "--others", "--exclude-standard"}},
		{"Total", []string{"diff", "HEAD", "--color=always", "--shortstat"}},
		{"Staged changes", []string{"diff", "--cached", "--color=always"}},
		{"Unstaged changes", []string{"diff", "--color=always"}},
	} {
		// A section that fails, such as the total in a repository without
		// commits, is left out.
		if out, err := git(gitTimeout, dir, s.args...); err == nil && out != "" {
			b.WriteString("\x1b[1m" + s.title + "\x1b[0m\n" + out + "\n\n")
		}
	}
	if b.Len() == 0 {
		return "No uncommitted changes\n", nil
	}
	return b.String(), nil
}
