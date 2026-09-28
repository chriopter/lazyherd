package herdr

import (
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/chriopter/lazyherd/internal/repo"
)

func TestFollowStartsLazygitPerSelection(t *testing.T) {
	bin := t.TempDir()
	log := filepath.Join(bin, "calls.log")
	script := "#!/bin/sh\necho \"$2\" >> " + log + "\nexec sleep 5\n"
	if err := os.WriteFile(filepath.Join(bin, "lazygit"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bin, "stty"), []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))

	reader, writer := io.Pipe()
	errC := make(chan error, 1)
	go func() { errC <- followSelections(reader, make(chan os.Signal)) }()
	for _, dir := range []string{"/repo/one", "/repo/two", "/repo/two"} {
		if err := SendSelection(writer, dir); err != nil {
			t.Fatal(err)
		}
		time.Sleep(200 * time.Millisecond)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}

	select {
	case err := <-errC:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("follower did not exit after the connection closed")
	}
	got, err := os.ReadFile(log)
	if err != nil {
		t.Fatal(err)
	}
	if lines := strings.Fields(string(got)); len(lines) != 2 || lines[0] != "/repo/one" || lines[1] != "/repo/two" {
		t.Fatalf("lazygit calls: %q", got)
	}
}

func TestFollowShowsChangesInPager(t *testing.T) {
	bin := t.TempDir()
	log := filepath.Join(bin, "calls.log")
	scriptOnPath(t, "lazygit", "echo lazygit \"$2\" >> "+log+"\nexec sleep 5\n")
	scriptOnPath(t, "less", "echo less \"$@\" >> "+log+"\ncat >> "+log+"\nexec sleep 5\n")
	scriptOnPath(t, "stty", "exit 0\n")
	dir := filepath.Join(t.TempDir(), "r")
	for _, args := range [][]string{{"init", "-q", dir}, {"-C", dir, "-c", "user.name=t", "-c", "user.email=t@t", "commit", "-q", "--allow-empty", "-m", "the subject"}} {
		if out, err := exec.Command("git", args...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v %s", args, err, out)
		}
	}

	var p lazygitProcess
	defer p.stop()
	if err := p.open(dir); err != nil {
		t.Fatal(err)
	}
	time.Sleep(200 * time.Millisecond)
	var buf strings.Builder
	if err := SendChange(&buf, dir, "HEAD"); err != nil {
		t.Fatal(err)
	}
	if err := p.open(strings.TrimSuffix(buf.String(), "\n")); err != nil {
		t.Fatal(err)
	}
	time.Sleep(300 * time.Millisecond)
	got, _ := os.ReadFile(log)
	if s := string(got); !strings.Contains(s, "lazygit "+dir) || !strings.Contains(s, "less -R") || !strings.Contains(s, "the subject") {
		t.Fatalf("calls: %q", s)
	}
	if err := p.open(repo.WorkTree + "\x1f" + dir); err != nil {
		t.Fatal(err)
	}
	if err := p.open("nope\x1f" + dir); err == nil {
		t.Fatal("unknown commit should fail")
	}
}
