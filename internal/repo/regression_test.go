package repo

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestParseStatusBranchEdges(t *testing.T) {
	for _, tt := range []struct {
		name, input string
		want        Status
	}{
		{"empty", "", Status{Branch: "@", NoUpstream: true}},
		{"initial", "# branch.oid (initial)\n# branch.head main\n", Status{Head: "(initial)", Branch: "main", NoUpstream: true}},
		{"detached short", "# branch.oid abc\n# branch.head (detached)", Status{Head: "abc", Branch: "@abc", NoUpstream: true}},
		{"ahead", "# branch.head main\n# branch.ab +12 -0", Status{Branch: "main", Ahead: 12}},
		{"behind", "# branch.head main\n# branch.ab +0 -23", Status{Branch: "main", Behind: 23}},
		{"unknown headers", "# future.field ignored\n# branch.head topic\n! ignored", Status{Branch: "topic", NoUpstream: true}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := parseStatus(tt.input); !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("got %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestParseStatusPathsAndConflicts(t *testing.T) {
	for _, tt := range []struct{ line, path, code string }{
		{"2 R. N... 100644 100644 100644 a b R100 new path/日本語.go\told path.go", "new path/日本語.go", "R "},
		{"1 .M N... 100644 100644 100644 a b path with  many spaces é.go", "path with  many spaces é.go", " M"},
		{"? untracked 日本語 with spaces", "untracked 日本語 with spaces", "??"},
		{"u UU N... 100644 100644 100644 100644 a b c conflict with spaces", "conflict with spaces", "UU"},
		{"u AA N... 100644 100644 100644 100644 a b c added", "added", "AA"},
		{"u DU N... 100644 100644 100644 100644 a b c deleted", "deleted", "DU"},
	} {
		t.Run(tt.code+tt.path, func(t *testing.T) {
			st := parseStatus(tt.line)
			if len(st.Changes) != 1 {
				t.Fatalf("got %+v", st)
			}
			c := st.Changes[0]
			if c.Path != tt.path || c.Code() != tt.code {
				t.Fatalf("got %+v (%q)", c, c.Code())
			}
		})
	}
}

func TestParseChangeRejectsIncompleteEntries(t *testing.T) {
	for _, line := range []string{"1", "2 R", "u U", "1 MM N...", "2 R. N... 1 2 3 a b", "u UU N... 1 2 3 4 a b c"} {
		if c, ok := parseChange(line); ok {
			t.Errorf("accepted %q: %+v", line, c)
		}
	}
}

func fakeGit(t *testing.T, script string) {
	t.Helper()
	bin := t.TempDir()
	if err := os.WriteFile(filepath.Join(bin, "git"), []byte("#!/bin/sh\n"+script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func TestScanOrderingAndOneStatusCall(t *testing.T) {
	root := t.TempDir()
	names := []string{"z-dirty", "a-dirty", "z-ahead", "a-behind", "b-clean", "a-clean", "failed"}
	for _, n := range names {
		if err := os.MkdirAll(filepath.Join(root, n, ".git"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("CALLS", t.TempDir())
	fakeGit(t, `name=${3##*/}
printf '%s\n' "$@" >> "$CALLS/$name"
case "$name" in
 z-dirty) printf '? a\n? b\n' ;;
 a-dirty) printf '? a\n# branch.ab +99 -0\n' ;;
 z-ahead) printf '# branch.ab +2 -0\n' ;;
 a-behind) printf '# branch.ab +0 -2\n' ;;
 failed) exit 1 ;;
esac
`)
	got, err := Scan(root)
	if err != nil {
		t.Fatal(err)
	}
	var order []string
	for _, r := range got {
		order = append(order, r.Name)
		if (r.Err != nil) != (r.Name == "failed") {
			t.Errorf("unexpected status error: %+v", r)
		}
		raw, err := os.ReadFile(filepath.Join(os.Getenv("CALLS"), r.Name))
		if err != nil {
			t.Fatal(err)
		}
		want := "--no-optional-locks\n-C\n" + filepath.Join(root, r.Name) + "\nstatus\n--porcelain=v2\n--branch\n--untracked-files=all\n-z\n"
		if string(raw) != want {
			t.Errorf("calls for %s: %q", r.Name, raw)
		}
	}
	want := []string{"z-dirty", "a-dirty", "a-behind", "z-ahead", "a-clean", "b-clean", "failed"}
	if !reflect.DeepEqual(order, want) {
		t.Fatalf("order %v, want %v", order, want)
	}
}

func TestScanSkipsSymlinksAndIncludesWorktrees(t *testing.T) {
	needGit(t)
	root := t.TempDir()
	source := filepath.Join(t.TempDir(), "source")
	run(t, root, "init", "-q", "-b", "main", source)
	run(t, source, "-c", "commit.gpgsign=false", "commit", "--allow-empty", "-qm", "initial")
	work := filepath.Join(root, "work")
	run(t, source, "worktree", "add", "-q", "-b", "work", work)
	if err := os.Symlink(source, filepath.Join(root, "linked")); err != nil {
		t.Fatal(err)
	}
	run(t, root, "init", "-q", filepath.Join(root, "container", "nested"))
	got, err := Scan(root)
	if err != nil || len(got) != 1 || got[0].Name != "work" || got[0].Branch != "work" || got[0].Err != nil {
		t.Fatalf("scan: %+v, %v", got, err)
	}
}

func TestFetchAllReturnsSortedFailureNames(t *testing.T) {
	fakeGit(t, `test "$4 $5 $6" = 'fetch --all --quiet' || exit 2
case "${3##*/}" in a|z) exit 1 ;; esac
`)
	got := FetchAll(t.TempDir(), []Repo{{Name: "z"}, {Name: "ok"}, {Name: "a"}})
	if !reflect.DeepEqual(got, []string{"a", "z"}) {
		t.Fatalf("failed: %v", got)
	}
	if got := FetchAll(t.TempDir(), nil); len(got) != 0 {
		t.Fatal(got)
	}
}

func syncFixture(t *testing.T) (string, string, string) {
	t.Helper()
	needGit(t)
	t.Setenv("GIT_CONFIG_GLOBAL", "/dev/null")
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	root := t.TempDir()
	remote := filepath.Join(root, "remote.git")
	work := filepath.Join(root, "work")
	run(t, root, "init", "-q", "--bare", "-b", "main", remote)
	run(t, root, "clone", "-q", remote, work)
	run(t, work, "commit", "--allow-empty", "-qm", "base")
	run(t, work, "push", "-qu", "origin", "main")
	return root, remote, work
}

func TestSyncRejectsDivergedRemote(t *testing.T) {
	root, remote, work := syncFixture(t)
	other := filepath.Join(root, "other")
	run(t, root, "clone", "-q", remote, other)
	run(t, work, "commit", "--allow-empty", "-qm", "local")
	run(t, other, "commit", "--allow-empty", "-qm", "remote")
	run(t, other, "push", "-q")
	before, err := status(work)
	if err != nil {
		t.Fatal(err)
	}
	result := Sync(root, Repo{Name: "work", Status: before})
	if result.Err == nil || !strings.HasPrefix(result.Err.Error(), "pull: ") || result.Pulled || result.Pushed {
		t.Fatalf("result: %+v", result)
	}
	after, err := status(work)
	if err != nil {
		t.Fatal(err)
	}
	if after.Head != before.Head || after.Ahead != 1 || after.Behind != 1 {
		t.Fatalf("divergence changed: %+v", after)
	}
}

func TestSyncReportsPushFailure(t *testing.T) {
	root, remote, work := syncFixture(t)
	run(t, work, "commit", "--allow-empty", "-qm", "local")
	if err := os.WriteFile(filepath.Join(remote, "hooks", "pre-receive"), []byte("#!/bin/sh\nexit 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	st, err := status(work)
	if err != nil {
		t.Fatal(err)
	}
	result := Sync(root, Repo{Name: "work", Status: st})
	if result.Err == nil || !strings.HasPrefix(result.Err.Error(), "push: ") || result.Pushed || result.Pulled {
		t.Fatalf("result: %+v", result)
	}
	after, err := status(work)
	if err != nil || after.Ahead != 1 {
		t.Fatalf("status: %+v %v", after, err)
	}
}

func TestSyncPullsRemoteCommit(t *testing.T) {
	root, remote, work := syncFixture(t)
	other := filepath.Join(root, "other")
	run(t, root, "clone", "-q", remote, other)
	run(t, other, "commit", "--allow-empty", "-qm", "remote")
	run(t, other, "push", "-q")
	st, err := status(work)
	if err != nil {
		t.Fatal(err)
	}
	res := Sync(root, Repo{Name: "work", Status: st})
	if res.Err != nil || !res.Pulled || res.Pushed {
		t.Fatalf("result: %+v", res)
	}
}

func TestSyncAllSkipsUnknownAndUntrackedUpstreams(t *testing.T) {
	fakeGit(t, "exit 99\n")
	got := SyncAll(t.TempDir(), []Repo{{Name: "bad", Err: errors.New("status failed")}, {Name: "new", Status: Status{NoUpstream: true}}})
	want := []SyncResult{{Name: "bad"}, {Name: "new"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v", got)
	}
}

func TestParallelBoundsAndCompletesEveryRepo(t *testing.T) {
	repos := make([]Repo, maxParallel*3+1)
	entered := make(chan int, len(repos))
	release := make(chan struct{})
	done := make(chan struct{})
	var mu sync.Mutex
	active, peak := 0, 0
	seen := make([]int, len(repos))
	go func() {
		parallel(repos, func(i int, r Repo) {
			mu.Lock()
			active++
			if active > peak {
				peak = active
			}
			seen[i]++
			mu.Unlock()
			entered <- i
			<-release
			mu.Lock()
			active--
			mu.Unlock()
		})
		close(done)
	}()
	defer closeIfOpen(release)
	for i := 0; i < maxParallel; i++ {
		select {
		case <-entered:
		case <-time.After(3 * time.Second):
			t.Fatal("workers failed to start")
		}
	}
	close(release)
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("workers failed to finish")
	}
	if peak != maxParallel {
		t.Errorf("peak = %d, want %d", peak, maxParallel)
	}
	for i, n := range seen {
		if n != 1 {
			t.Errorf("repo %d called %d times", i, n)
		}
	}
	parallel(nil, func(int, Repo) { t.Error("called for empty list") })
}

func closeIfOpen(c chan struct{}) {
	select {
	case <-c:
	default:
		close(c)
	}
}

func TestKnownBugStatusDecodesGitQuotedPaths(t *testing.T) {
	needGit(t)
	root := t.TempDir()
	run(t, root, "init", "-q", "-b", "main")
	path := "日本語\tfile"
	if err := os.WriteFile(filepath.Join(root, path), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	st, err := status(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(st.Changes) != 1 || st.Changes[0].Path != path {
		t.Fatalf("quoted path not decoded: %+v", st.Changes)
	}
}

func TestSyncReportsStatusFailureAfterPull(t *testing.T) {
	fakeGit(t, `case "$4" in pull) exit 0 ;; *) exit 1 ;; esac`)
	result := Sync(t.TempDir(), Repo{Name: "repo"})
	if result.Err == nil || result.Pulled || result.Pushed {
		t.Fatalf("result: %+v", result)
	}
}

func TestGitTimeout(t *testing.T) {
	fakeGit(t, "exec sleep 30\n")
	started := time.Now()
	if _, err := git(20*time.Millisecond, t.TempDir(), "status"); err == nil {
		t.Fatal("hanging git succeeded")
	}
	if elapsed := time.Since(started); elapsed > 3*time.Second {
		t.Fatalf("timeout took %v", elapsed)
	}
}
