// Package follow implements the companion pane: a process that keeps lazygit
// open for whatever repository the lazyherd list currently selects.
package follow

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
	"time"
)

// resetTerminal leaves the alternate screen, shows the cursor and disables
// mouse reporting, in case lazygit was killed before it could.
const resetTerminal = "\x1b[?1000l\x1b[?1002l\x1b[?1003l\x1b[?1006l\x1b[?1049l\x1b[?25h\x1b[0m"

// Run listens on a Unix socket for repository paths, one per line, and runs
// lazygit for the latest one. It returns when the sending side closes.
func Run(socket string) error {
	_ = os.Remove(socket)
	ln, err := net.Listen("unix", socket)
	if err != nil {
		return err
	}
	defer ln.Close()
	defer os.Remove(socket)

	fmt.Print("lazyherd: waiting for a selection …\n")
	conn, err := ln.Accept()
	if err != nil {
		return err
	}
	defer conn.Close()

	var f follower
	defer f.stop()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)
	lines := make(chan string)
	go func() {
		sc := bufio.NewScanner(conn)
		for sc.Scan() {
			lines <- sc.Text()
		}
		close(lines)
	}()
	for {
		select {
		case dir, ok := <-lines:
			if !ok {
				return nil // lazyherd went away
			}
			if err := f.open(dir); err != nil {
				fmt.Fprintf(os.Stderr, "lazyherd: %v\n", err)
			}
		case <-quit:
			return nil
		}
	}
}

// follower owns at most one running lazygit.
type follower struct {
	cmd  *exec.Cmd
	done chan error
	dir  string
}

func (f *follower) open(dir string) error {
	if f.cmd != nil && f.dir == dir && f.running() {
		return nil
	}
	f.stop()
	cmd := exec.Command("lazygit", "-p", dir)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	if err := cmd.Start(); err != nil {
		return err
	}
	f.cmd, f.dir, f.done = cmd, dir, make(chan error, 1)
	go func(done chan error) { done <- cmd.Wait() }(f.done)
	return nil
}

func (f *follower) running() bool {
	select {
	case err := <-f.done:
		f.done <- err // keep it readable for stop
		return false
	default:
		return true
	}
}

// stop terminates the running lazygit and restores the terminal.
func (f *follower) stop() {
	if f.cmd == nil {
		return
	}
	if f.running() {
		_ = f.cmd.Process.Signal(syscall.SIGTERM)
		select {
		case <-f.done:
		case <-time.After(2 * time.Second):
			_ = f.cmd.Process.Kill()
			<-f.done
		}
	} else {
		<-f.done
	}
	f.cmd = nil
	fmt.Print(resetTerminal)
	sane := exec.Command("stty", "sane")
	sane.Stdin = os.Stdin
	_ = sane.Run()
}

// Send writes a repository path to a running follower.
func Send(conn io.Writer, dir string) error {
	_, err := fmt.Fprintln(conn, dir)
	return err
}

// Dial connects to the follower's socket, waiting for it to appear.
func Dial(socket string, wait time.Duration) (net.Conn, error) {
	deadline := time.Now().Add(wait)
	for {
		conn, err := net.Dial("unix", socket)
		if err == nil {
			return conn, nil
		}
		if time.Now().After(deadline) {
			return nil, errors.New("companion pane did not start")
		}
		time.Sleep(100 * time.Millisecond)
	}
}
