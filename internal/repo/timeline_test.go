package repo

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTimelineMergesReposNewestFirst(t *testing.T) {
	needGit(t)
	root := t.TempDir()
	commit := func(dir, msg, date string) {
		t.Helper()
		os.WriteFile(filepath.Join(dir, msg), []byte(msg), 0o644)
		run(t, dir, "add", msg)
		t.Setenv("GIT_COMMITTER_DATE", date)
		run(t, dir, "commit", "-q", "-m", msg, "--date", date)
	}
	for _, name := range []string{"a", "b", "empty"} {
		run(t, root, "init", "-q", "-b", "main", filepath.Join(root, name))
	}
	commit(filepath.Join(root, "a"), "a1", "2024-01-01T10:00:00")
	commit(filepath.Join(root, "b"), "b1", "2024-01-03T10:00:00")
	commit(filepath.Join(root, "a"), "a2", "2024-01-02T10:00:00")

	repos := []Repo{{Name: "a"}, {Name: "b"}, {Name: "empty"}}
	got := Timeline(root, repos, 10, 10)
	var order []string
	for _, c := range got {
		order = append(order, c.Repo+":"+c.Subject)
	}
	if want := "b:b1 a:a2 a:a1"; strings.Join(order, " ") != want {
		t.Fatalf("timeline %v, want %s", order, want)
	}
	if got[0].Hash == "" || got[0].Time == 0 || got[0].Author != "t" {
		t.Errorf("commit fields: %+v", got[0])
	}
	if got := Timeline(root, repos, 1, 10); len(got) != 2 {
		t.Errorf("perRepo 1 gave %d commits", len(got))
	}
	if got := Timeline(root, repos, 10, 2); len(got) != 2 || got[1].Subject != "a2" {
		t.Errorf("limit 2 gave %+v", got)
	}
}

func TestChangesShowsCommitAndWorkTree(t *testing.T) {
	needGit(t)
	dir := filepath.Join(t.TempDir(), "r")
	run(t, filepath.Dir(dir), "init", "-q", "-b", "main", dir)
	if got, err := Changes(dir, WorkTree+":v1", 80); err != nil || got != "No uncommitted changes\n" {
		t.Fatalf("empty repo: %q %v", got, err)
	}
	os.WriteFile(filepath.Join(dir, "a"), []byte("one\ntwo\n"), 0o644)
	run(t, dir, "add", "a")
	run(t, dir, "commit", "-q", "-m", "add a")

	got, err := Changes(dir, "HEAD", 80)
	if err != nil || !strings.Contains(got, "add a") || !strings.Contains(got, "1 file changed, 2 insertions(+)") || !strings.Contains(got, "two") {
		t.Fatalf("commit view: %q %v", got, err)
	}

	os.WriteFile(filepath.Join(dir, "a"), []byte("one\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "b"), []byte("b\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "new"), []byte("n\n"), 0o644)
	run(t, dir, "add", "b")
	got, err = Changes(dir, WorkTree, 80)
	for _, want := range []string{"Staged", "Unstaged", "Untracked", "new", "Total", "2 files changed", "two", "b"} {
		if err != nil || !strings.Contains(got, want) {
			t.Fatalf("worktree view lacks %q: %q %v", want, got, err)
		}
	}
	if _, err := Changes(dir, "nope", 80); err == nil {
		t.Fatal("unknown commit should fail")
	}
}
