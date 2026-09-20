package herdr

import (
	"bufio"
	"context"
	"errors"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"syscall"
	"testing"
	"time"
)

func scriptOnPath(t *testing.T, name, script string) {
	t.Helper()
	bin := t.TempDir()
	if err := os.WriteFile(filepath.Join(bin, name), []byte("#!/bin/sh\n"+script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("HERDR_BIN_PATH", "") // the fake must win even when the tests run in a Herdr pane
}

func TestLoadExcludesRootAndOutsideAndFallsBackToWorkspaceID(t *testing.T) {
	root := t.TempDir()
	fakeHerdr(t, root)
	t.Setenv("HERDR_WORKSPACE_ID", "w2")
	st, err := Load(root)
	if err != nil || st.WorkspaceLabel() != "w2" {
		t.Fatalf("state: %+v, %v", st, err)
	}
	if len(st.panes) != 2 || len(st.panes["api"]) != 1 || len(st.panes["web"]) != 1 {
		t.Fatalf("mapped root or outside pane: %+v", st.panes)
	}
	if p := st.Pane("api", ""); p == nil || p.Cwd != filepath.Join(root, "api", "internal") {
		t.Fatalf("nested pane: %+v", p)
	}
	if st.Pane("api", "w2") != nil || st.Pane("missing", "") != nil {
		t.Fatal("wrong workspace or missing repo matched")
	}
}

func TestLoadMalformedResponses(t *testing.T) {
	for _, out := range []string{"{", `{"result":[]}`, `{"result":{"panes":"bad"}}`, `{}`} {
		t.Run(out, func(t *testing.T) {
			scriptOnPath(t, "herdr", "printf '%s' '"+out+"'\n")
			t.Setenv("HERDR_WORKSPACE_ID", "w")
			st, err := Load(t.TempDir())
			if err == nil || len(st.panes) != 0 || st.WorkspaceLabel() != "w" {
				t.Fatalf("got %+v, %v", st, err)
			}
		})
	}
}

func TestLoadKeepsPanesWhenWorkspaceLookupFails(t *testing.T) {
	scriptOnPath(t, "herdr", `case "$1" in pane) printf '%s' '{"result":{"panes":[{"pane_id":"p","cwd":"/repos/a","workspace_id":"w"}]}}';; *) exit 1;; esac`)
	t.Setenv("HERDR_WORKSPACE_ID", "w")
	st, err := Load("/repos")
	if err != nil || st.Pane("a", "w") == nil || st.WorkspaceLabel() != "w" {
		t.Fatalf("got %+v, %v", st, err)
	}
}

func TestLoadHangingCommand(t *testing.T) {
	t.Skip("herdr timeout is a fixed five-second constant; no short test timeout seam is exposed")
}

func TestLazygitProcessReusesAndRestarts(t *testing.T) {
	scriptOnPath(t, "lazygit", "exec sleep 30\n")
	scriptOnPath(t, "stty", "exit 0\n")
	var p lazygitProcess
	defer p.stop()
	if err := p.open("/one"); err != nil {
		t.Fatal(err)
	}
	first := p.cmd
	done := p.done
	if err := p.open("/one"); err != nil {
		t.Fatal(err)
	}
	if p.cmd != first {
		t.Fatal("same directory restarted a running process")
	}
	if err := p.open("/two"); err != nil {
		t.Fatal(err)
	}
	if p.cmd == first || p.dir != "/two" {
		t.Fatal("new directory did not restart")
	}
	select {
	case <-done:
	default:
		t.Fatal("old process not reaped")
	}
	// Even the same directory must restart when lazygit has exited on its own.
	if err := p.cmd.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-p.done:
	case <-time.After(3 * time.Second):
		t.Fatal("process did not exit")
	}
	second := p.cmd
	if err := p.open("/two"); err != nil {
		t.Fatal(err)
	}
	if p.cmd == second {
		t.Fatal("exited process reused")
	}
}

// Run Follow in a subprocess so a real SIGTERM cannot kill the test runner.
func TestFollowHelperProcess(t *testing.T) {
	if os.Getenv("LAZYHERD_FOLLOW_HELPER") != "1" {
		return
	}
	if err := Follow(os.Getenv("LAZYHERD_TEST_SOCKET")); err != nil {
		t.Fatal(err)
	}
}

func startFollower(t *testing.T, socket string) <-chan error {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestFollowHelperProcess$")
	cmd.Env = append(os.Environ(), "LAZYHERD_FOLLOW_HELPER=1", "LAZYHERD_TEST_SOCKET="+socket)
	if err := cmd.Start(); err != nil {
		cancel()
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	t.Cleanup(func() { cancel(); _ = cmd.Process.Kill() })
	return done
}

func awaitFile(t *testing.T, path string, want string) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for {
		b, err := os.ReadFile(path)
		if err == nil && strings.Contains(string(b), want) {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("waiting for %q in %s: %q, %v", want, path, b, err)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestFollowSocketCloseAndSIGTERM(t *testing.T) {
	requireUnixSockets(t)
	for _, signalExit := range []bool{false, true} {
		name := "close"
		if signalExit {
			name = "SIGTERM"
		}
		t.Run(name, func(t *testing.T) {
			dir := shortDir(t)
			socket := filepath.Join(dir, "f.sock")
			log := filepath.Join(dir, "log")
			t.Setenv("FOLLOW_LOG", log)
			// Signal only after lazygit starts: Follow installs its handler after Accept.
			body := "printf '%s\\n' \"$2\" >> \"$FOLLOW_LOG\"\n"
			if signalExit {
				body += "kill -TERM \"$PPID\"\n"
			}
			scriptOnPath(t, "lazygit", body+"exec sleep 30\n")
			scriptOnPath(t, "stty", "exit 0\n")
			scriptOnPath(t, "herdr", `echo "$*" >> "$FOLLOW_LOG"`)
			t.Setenv("HERDR_PANE_ID", "own")
			done := startFollower(t, socket)
			conn, err := dial(socket, 3*time.Second)
			if err != nil {
				t.Fatal(err)
			}
			defer conn.Close()
			if err := SendSelection(conn, "/repo with spaces/日本語"); err != nil {
				t.Fatal(err)
			}
			awaitFile(t, log, "/repo with spaces/日本語\n")
			if !signalExit {
				if err := conn.Close(); err != nil {
					t.Fatal(err)
				}
			}
			select {
			case err := <-done:
				if err != nil {
					t.Fatal(err)
				}
			case <-time.After(4 * time.Second):
				t.Fatal("Follow did not exit")
			}
			if _, err := os.Stat(socket); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("socket remains: %v", err)
			}
			awaitFile(t, log, "pane close own\n")
		})
	}
}

func TestStartCompanionProtocol(t *testing.T) {
	requireUnixSockets(t)
	dir := shortDir(t)
	socket := filepath.Join(dir, "lazyherd-own.sock")
	t.Setenv("XDG_RUNTIME_DIR", dir)
	log := filepath.Join(dir, "calls")
	t.Setenv("HERDR_CALLS", log)
	scriptOnPath(t, "herdr", `printf '%s\n' "$@" >> "$HERDR_CALLS"
case "$1 $2" in
 'pane split') printf '%s' '{"result":{"pane":{"pane_id":"companion"}}}' ;;
 'pane run') exit 0 ;;
 *) exit 1 ;;
esac
`)
	listener, err := net.Listen("unix", socket)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	pane, conn, err := StartCompanion("/repos with spaces", "own", 0.3)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if pane != "companion" {
		t.Fatalf("pane %q", pane)
	}
	peer, err := listener.Accept()
	if err != nil {
		t.Fatal(err)
	}
	defer peer.Close()
	if err := conn.SetDeadline(time.Now().Add(3 * time.Second)); err != nil {
		t.Fatal(err)
	}
	if err := peer.SetDeadline(time.Now().Add(3 * time.Second)); err != nil {
		t.Fatal(err)
	}
	if err := SendSelection(conn, "/repos with spaces/api"); err != nil {
		t.Fatal(err)
	}
	got, err := bufio.NewReader(peer).ReadString('\n')
	if err != nil || got != "/repos with spaces/api\n" {
		t.Fatalf("selection: %q %v", got, err)
	}
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(log)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"pane", "split", "--pane", "own", "--direction", "right", "--ratio", "0.30", "--cwd", "/repos with spaces", "--no-focus", "pane", "run", "companion", exe + " follow " + socket}
	if !reflect.DeepEqual(strings.Split(strings.TrimSuffix(string(raw), "\n"), "\n"), want) {
		t.Fatalf("calls: %s", raw)
	}
}

func TestStartCompanionClosesPaneOnRunFailure(t *testing.T) {
	log := filepath.Join(t.TempDir(), "calls")
	t.Setenv("HERDR_CALLS", log)
	scriptOnPath(t, "herdr", `echo "$*" >> "$HERDR_CALLS"
case "$1 $2" in
 'pane split') printf '%s' '{"result":{"pane":{"pane_id":"child"}}}' ;;
 'pane run') exit 1 ;;
 'pane close') exit 0 ;;
esac
`)
	pane, conn, err := StartCompanion(t.TempDir(), "own", 0.3)
	if err == nil || conn != nil || pane != "" {
		t.Fatalf("result: %q %v %v", pane, conn, err)
	}
	raw, e := os.ReadFile(log)
	if e != nil {
		t.Fatal(e)
	}
	if !strings.HasSuffix(string(raw), "pane close child\n") {
		t.Fatalf("pane not closed: %s", raw)
	}
}

func TestRunReportsHerdrStderr(t *testing.T) {
	scriptOnPath(t, "herdr", `echo "some noise" >&2; echo "error: unknown flag --no-focus" >&2; exit 1`)
	_, err := splitRight("own", t.TempDir(), 0.3)
	if err == nil || err.Error() != "herdr pane split: error: unknown flag --no-focus" {
		t.Fatalf("stderr not surfaced: %v", err)
	}
	scriptOnPath(t, "herdr", `echo '{"id":"cli:pane:split","error":{"code":"protocol_mismatch","message":"client protocol 20 is older than server protocol 22"}}' >&2; exit 1`)
	_, err = splitRight("own", t.TempDir(), 0.3)
	if err == nil || err.Error() != "herdr pane split: client protocol 20 is older than server protocol 22" {
		t.Fatalf("JSON error not unwrapped: %v", err)
	}
	scriptOnPath(t, "herdr", `exit 1`)
	if err := runInPane("own", "true"); err == nil || err.Error() != "herdr pane run: exit status 1" {
		t.Fatalf("silent failure: %v", err)
	}
}

func TestSplitRejectsMissingPaneID(t *testing.T) {
	scriptOnPath(t, "herdr", `printf '%s' '{"result":{"pane":{}}}'`)
	if _, err := splitRight("own", t.TempDir(), 0.3); err == nil {
		t.Fatal("missing pane ID accepted")
	}
}

func TestDialAndSelectionErrors(t *testing.T) {
	if conn, err := dial(filepath.Join(t.TempDir(), "missing"), 0); err == nil || conn != nil {
		t.Fatal("missing socket accepted")
	}
	a, b := net.Pipe()
	a.Close()
	b.Close()
	if err := SendSelection(a, "/repo"); err == nil {
		t.Fatal("closed connection accepted")
	}
	if err := Follow(filepath.Join(t.TempDir(), "missing", "socket")); err == nil {
		t.Fatal("invalid socket path accepted")
	}
}

// Some restricted runners prohibit AF_UNIX. Skip only that platform restriction;
// all other socket failures remain failures (including path length and timeouts).
// shortDir returns a temp dir whose socket paths stay under the Unix socket
// path limit, which t.TempDir() exceeds on macOS.
func shortDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("/tmp", "lh")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	return dir
}

func requireUnixSockets(t *testing.T) {
	t.Helper()
	listener, err := net.Listen("unix", filepath.Join(shortDir(t), "probe.sock"))
	if errors.Is(err, syscall.EPERM) || errors.Is(err, syscall.EACCES) {
		t.Skipf("runner prohibits Unix sockets: %v", err)
	}
	if err != nil {
		t.Fatal(err)
	}
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestFollowSelectionsStopsOnSignal(t *testing.T) {
	log := filepath.Join(t.TempDir(), "started")
	t.Setenv("FOLLOW_LOG", log)
	scriptOnPath(t, "lazygit", "printf started > \"$FOLLOW_LOG\"\nexec sleep 30\n")
	scriptOnPath(t, "stty", "exit 0\n")
	reader, writer := io.Pipe()
	defer reader.Close()
	defer writer.Close()
	quit := make(chan os.Signal, 1)
	done := make(chan error, 1)
	go func() { done <- followSelections(reader, quit) }()
	if err := SendSelection(writer, "/one"); err != nil {
		t.Fatal(err)
	}
	awaitFile(t, log, "started")
	quit <- syscall.SIGTERM
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(4 * time.Second):
		t.Fatal("signal did not stop follower")
	}
	// Unblock the scanner before returning, including when quit wins the select.
	writer.Close()
}

func TestHerdrActionArguments(t *testing.T) {
	log := filepath.Join(t.TempDir(), "calls")
	t.Setenv("HERDR_CALLS", log)
	scriptOnPath(t, "herdr", `printf '%s\n' "$@" >> "$HERDR_CALLS"`)
	for _, action := range []func() error{
		func() error { return ClosePane("p") }, func() error { return FocusRight("own") }, func() error { return FocusTab("tab") },
		func() error { return CreateTab("w", "/with spaces", "label") }, func() error { return CreateTab("", "/repo", "api") },
	} {
		if err := action(); err != nil {
			t.Fatal(err)
		}
	}
	raw, err := os.ReadFile(log)
	if err != nil {
		t.Fatal(err)
	}
	want := "pane\nclose\np\npane\nfocus\n--direction\nright\n--pane\nown\ntab\nfocus\ntab\ntab\ncreate\n--cwd\n/with spaces\n--label\nlabel\n--focus\n--workspace\nw\ntab\ncreate\n--cwd\n/repo\n--label\napi\n--focus\n"
	if string(raw) != want {
		t.Fatalf("arguments: %q", raw)
	}
}
