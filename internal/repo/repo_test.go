package repo

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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

func TestStatusParsesChanges(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	dir := t.TempDir()
	run(t, dir, "init", "-q", "-b", "main", ".")
	os.WriteFile(filepath.Join(dir, "a.txt"), []byte("a"), 0o644)
	run(t, dir, "add", "a.txt")
	run(t, dir, "commit", "-q", "-m", "init")
	os.WriteFile(filepath.Join(dir, "a.txt"), []byte("aa"), 0o644)
	os.MkdirAll(filepath.Join(dir, "sub"), 0o755)
	os.WriteFile(filepath.Join(dir, "sub", "new.go"), []byte("x"), 0o644)
	run(t, dir, "add", "sub/new.go")

	branch, changes, err := Status(dir)
	if err != nil {
		t.Fatal(err)
	}
	if branch != "main" {
		t.Errorf("branch line: %q", branch)
	}
	if len(changes) != 2 || changes[0].Path != "a.txt" || changes[0].Code() != " M" ||
		changes[1].Path != "sub/new.go" || changes[1].Code() != "A " {
		t.Errorf("changes: %+v", changes)
	}
	if d := Diff(dir, changes[0]); !strings.Contains(d, "aa") {
		t.Errorf("unstaged diff: %q", d)
	}
	if d := Diff(dir, changes[1]); !strings.HasPrefix(stripANSI(d), "@@") {
		t.Errorf("staged diff: %q", d)
	}
}

func TestScanMissingRoot(t *testing.T) {
	if _, err := Scan(filepath.Join(t.TempDir(), "nope")); err == nil {
		t.Fatal("want error for missing root")
	}
}

func stripANSI(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == 0x1b {
			for i < len(s) && s[i] != 'm' {
				i++
			}
			continue
		}
		b.WriteByte(s[i])
	}
	return b.String()
}
