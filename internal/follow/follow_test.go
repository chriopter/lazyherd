package follow

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRunStartsLazygitPerSelection(t *testing.T) {
	bin := t.TempDir()
	log := filepath.Join(bin, "calls.log")
	script := "#!/bin/sh\necho \"$2\" >> " + log + "\nexec sleep 5\n"
	os.WriteFile(filepath.Join(bin, "lazygit"), []byte(script), 0o755)
	os.WriteFile(filepath.Join(bin, "stty"), []byte("#!/bin/sh\nexit 0\n"), 0o755)
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))

	sock := filepath.Join(t.TempDir(), "s.sock")
	errc := make(chan error, 1)
	go func() { errc <- Run(sock) }()

	conn, err := Dial(sock, 3*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	for _, dir := range []string{"/repo/one", "/repo/two", "/repo/two"} { // same again: no restart
		Send(conn, dir)
		time.Sleep(200 * time.Millisecond) // let the fake lazygit log its start
	}
	conn.Close()

	select {
	case err := <-errc:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("follower did not exit after the connection closed")
	}
	got, _ := os.ReadFile(log)
	if lines := strings.Fields(string(got)); len(lines) != 2 || lines[0] != "/repo/one" || lines[1] != "/repo/two" {
		t.Fatalf("lazygit calls: %q", got)
	}
}
