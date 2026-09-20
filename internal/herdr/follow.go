package herdr

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

// resetTerminal repairs the terminal when lazygit cannot clean up after
// itself: mouse, focus and in-band resize reports off, main screen, cursor on.
const resetTerminal = "\x1b[?1000l\x1b[?1002l\x1b[?1003l\x1b[?1006l\x1b[?1004l\x1b[?2048l\x1b[?1049l\x1b[?25h\x1b[0m"

// Follow listens for repository paths and keeps lazygit open for the latest
// one. When the cockpit goes away, so does the pane this runs in: it was
// split off for lazygit alone, and a bare shell there would only linger.
func Follow(socket string) error {
	_ = os.Remove(socket)
	listener, err := net.Listen("unix", socket)
	if err != nil {
		return err
	}
	defer listener.Close()
	defer os.Remove(socket)
	if pane := os.Getenv("HERDR_PANE_ID"); pane != "" {
		defer func() { _ = ClosePane(pane) }()
	}

	fmt.Println("lazyherd: waiting for a selection …")
	conn, err := listener.Accept()
	if err != nil {
		return err
	}
	defer conn.Close()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)
	defer signal.Stop(quit)
	return followSelections(conn, quit)
}

func followSelections(input io.Reader, quit <-chan os.Signal) error {
	var process lazygitProcess
	defer process.stop()

	paths := make(chan string)
	go func() {
		scanner := bufio.NewScanner(input)
		for scanner.Scan() {
			paths <- scanner.Text()
		}
		close(paths)
	}()
	for {
		select {
		case dir, ok := <-paths:
			if !ok {
				return nil
			}
			if err := process.open(dir); err != nil {
				fmt.Fprintf(os.Stderr, "lazyherd: %v\n", err)
			}
		case <-quit:
			return nil
		}
	}
}

type lazygitProcess struct {
	cmd  *exec.Cmd
	done chan struct{}
	dir  string
}

func (p *lazygitProcess) open(dir string) error {
	if p.cmd != nil && p.dir == dir && p.running() {
		return nil
	}
	p.stop()
	cmd := exec.Command("lazygit", "-p", dir)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	if err := cmd.Start(); err != nil {
		return err
	}
	p.cmd, p.dir, p.done = cmd, dir, make(chan struct{})
	go func() {
		_ = cmd.Wait()
		close(p.done)
	}()
	return nil
}

func (p *lazygitProcess) running() bool {
	select {
	case <-p.done:
		return false
	default:
		return true
	}
}

func (p *lazygitProcess) stop() {
	if p.cmd == nil {
		return
	}
	if p.running() {
		_ = p.cmd.Process.Signal(syscall.SIGTERM)
		select {
		case <-p.done:
		case <-time.After(2 * time.Second):
			_ = p.cmd.Process.Kill()
			<-p.done
		}
	}
	p.cmd = nil
	fmt.Print(resetTerminal)
	sane := exec.Command("stty", "sane")
	sane.Stdin = os.Stdin
	_ = sane.Run()
}

// SendSelection tells the follower which repository lazygit should open.
func SendSelection(conn io.Writer, dir string) error {
	_, err := fmt.Fprintln(conn, dir)
	return err
}

// StartCompanion splits a pane, starts the follower, and connects to it.
func StartCompanion(root, ownPane string, ratio float64) (string, net.Conn, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", nil, err
	}
	pane, err := splitRight(ownPane, root, ratio)
	if err != nil {
		return "", nil, err
	}
	fail := func(err error) (string, net.Conn, error) {
		_ = ClosePane(pane)
		return "", nil, err
	}
	socket, err := socketPath(ownPane)
	if err != nil {
		return fail(err)
	}
	if err := runInPane(pane, exe+" follow "+socket); err != nil {
		return fail(err)
	}
	conn, err := dial(socket, 10*time.Second)
	if err != nil {
		return fail(err)
	}
	return pane, conn, nil
}

func dial(socket string, wait time.Duration) (net.Conn, error) {
	deadline := time.Now().Add(wait)
	for {
		conn, err := net.Dial("unix", socket)
		if err == nil {
			return conn, nil
		}
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("companion pane did not start: %w", err)
		}
		time.Sleep(100 * time.Millisecond)
	}
}

// socketPath picks a short path for the companion socket: Unix socket paths
// are limited to about 100 bytes, and macOS temp dirs alone can exceed that.
func socketPath(ownPane string) (string, error) {
	name := "lazyherd-" + strings.NewReplacer(":", "-", "/", "-").Replace(ownPane) + ".sock"
	for _, dir := range []string{os.Getenv("XDG_RUNTIME_DIR"), os.TempDir(), "/tmp"} {
		if dir == "" {
			continue
		}
		p := filepath.Join(dir, name)
		if len(p) < 100 {
			return p, nil
		}
	}
	return "", errors.New("no short enough directory for the companion socket")
}
