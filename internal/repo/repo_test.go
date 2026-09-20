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

func TestScan(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	root := t.TempDir()

	clean := filepath.Join(root, "clean")
	run(t, root, "init", "-q", "-b", "main", clean)
	os.WriteFile(filepath.Join(clean, "a"), []byte("a"), 0o644)
	run(t, clean, "add", "a")
	run(t, clean, "commit", "-q", "-m", "init")

	dirty := filepath.Join(root, "dirty")
	run(t, root, "init", "-q", "-b", "work", dirty)
	os.WriteFile(filepath.Join(dirty, "x"), []byte("x"), 0o644)
	os.WriteFile(filepath.Join(dirty, "y"), []byte("y"), 0o644)

	os.Mkdir(filepath.Join(root, "not-a-repo"), 0o755)

	repos, err := Scan(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(repos) != 2 {
		t.Fatalf("want 2 repos, got %d: %+v", len(repos), repos)
	}
	if repos[0].Name != "dirty" || repos[0].Changes != 2 || repos[0].Branch != "work" || !repos[0].NoUpstream {
		t.Errorf("dirty repo scanned wrong: %+v", repos[0])
	}
	if repos[1].Name != "clean" || repos[1].Changes != 0 || repos[1].Branch != "main" {
		t.Errorf("clean repo scanned wrong: %+v", repos[1])
	}
}

func TestScanMissingRoot(t *testing.T) {
	if _, err := Scan(filepath.Join(t.TempDir(), "nope")); err == nil {
		t.Fatal("want error for missing root")
	}
}
