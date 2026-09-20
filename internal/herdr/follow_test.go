package herdr

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
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
