package repo

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func run(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@example.com",
		"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@example.com",
		"GIT_CONFIG_GLOBAL=/dev/null")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func needGit(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
}

func TestScan(t *testing.T) {
	needGit(t)
	root := t.TempDir()

	clean := filepath.Join(root, "clean")
	run(t, root, "init", "-q", "-b", "main", clean)
	os.WriteFile(filepath.Join(clean, "a"), []byte("a"), 0o644)
	run(t, clean, "add", "a")
	run(t, clean, "commit", "-q", "-m", "init")

	dirty := filepath.Join(root, "dirty")
	run(t, root, "init", "-q", "-b", "work", dirty)
	os.MkdirAll(filepath.Join(dirty, "sub"), 0o755)
	os.WriteFile(filepath.Join(dirty, "x"), []byte("x"), 0o644)
	os.WriteFile(filepath.Join(dirty, "sub", "y"), []byte("y"), 0o644)

	os.Mkdir(filepath.Join(root, "not-a-repo"), 0o755)

	repos, err := Scan(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(repos) != 2 {
		t.Fatalf("want 2 repos, got %d: %+v", len(repos), repos)
	}
	d := repos[0]
	if d.Name != "dirty" || len(d.Changes) != 2 || d.Branch != "work" || !d.NoUpstream || d.Changes[0].Path != "sub/y" {
		t.Errorf("dirty repo scanned wrong: %+v", d)
	}
	if c := repos[1]; c.Name != "clean" || c.Dirty() || c.Branch != "main" || c.Head == "" {
		t.Errorf("clean repo scanned wrong: %+v", c)
	}
}

func TestScanMissingRoot(t *testing.T) {
	if _, err := Scan(filepath.Join(t.TempDir(), "nope")); err == nil {
		t.Fatal("want error for missing root")
	}
}

func TestSyncPullsAndPushes(t *testing.T) {
	needGit(t)
	root := t.TempDir()
	remote := filepath.Join(root, "remote.git")
	run(t, root, "init", "-q", "--bare", remote)
	dir := filepath.Join(root, "work")
	run(t, root, "clone", "-q", remote, dir)
	run(t, dir, "checkout", "-q", "-b", "main")
	os.WriteFile(filepath.Join(dir, "a"), []byte("a"), 0o644)
	run(t, dir, "add", "a")
	run(t, dir, "commit", "-q", "-m", "one")
	run(t, dir, "push", "-q", "-u", "origin", "main")
	os.WriteFile(filepath.Join(dir, "b"), []byte("b"), 0o644)
	run(t, dir, "add", "b")
	run(t, dir, "commit", "-q", "-m", "two") // now ahead by one

	repos, _ := Scan(root)
	var work Repo
	for _, r := range repos {
		if r.Name == "work" {
			work = r
		}
	}
	if work.Ahead != 1 {
		t.Fatalf("setup: want ahead 1, got %+v", work)
	}
	res := Sync(root, work)
	if res.Err != nil || res.Pulled || !res.Pushed {
		t.Fatalf("sync result: %+v", res)
	}
	repos, _ = Scan(root)
	for _, r := range repos {
		if r.Name == "work" && r.Ahead != 0 {
			t.Fatalf("still ahead after sync: %+v", r)
		}
	}
	if res := Sync(root, Repo{Name: "work", Status: Status{NoUpstream: true}}); res.Pulled || res.Pushed || res.Err != nil {
		t.Fatalf("no-upstream repo must be skipped: %+v", res)
	}
}
