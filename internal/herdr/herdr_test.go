package herdr

import (
	"os"
	"path/filepath"
	"testing"
)

// fakeHerdr puts a shell script named herdr on PATH that answers pane and
// workspace listings for the given root.
func fakeHerdr(t *testing.T, root string) {
	t.Helper()
	bin := t.TempDir()
	script := `#!/bin/sh
case "$1 $2" in
  "pane list") printf '%s' '{"result":{"panes":[
    {"pane_id":"p1","tab_id":"t1","workspace_id":"w1","cwd":"` + root + `/api/internal"},
    {"pane_id":"p2","tab_id":"t2","workspace_id":"w2","cwd":"` + root + `/web"},
    {"pane_id":"p3","tab_id":"t3","workspace_id":"w1","cwd":"` + root + `"},
    {"pane_id":"p4","tab_id":"t4","workspace_id":"w1","cwd":"/elsewhere"}]}}' ;;
  "workspace list") printf '%s' '{"result":{"workspaces":[{"workspace_id":"w1","label":"backend"}]}}' ;;
  *) exit 1 ;;
esac
`
	if err := os.WriteFile(filepath.Join(bin, "herdr"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func TestLoadMapsPanesToRepos(t *testing.T) {
	root := t.TempDir()
	fakeHerdr(t, root)
	t.Setenv("HERDR_WORKSPACE_ID", "w1")

	st := Load(root)
	if !st.Available {
		t.Fatal("expected herdr to be available")
	}
	if p := st.Pane("api", "w1"); p == nil || p.TabID != "t1" {
		t.Errorf("api pane in w1: got %+v", p)
	}
	if p := st.Pane("web", "w1"); p != nil {
		t.Errorf("web has no pane in w1, got %+v", p)
	}
	if p := st.Pane("web", ""); p == nil || p.TabID != "t2" {
		t.Errorf("web pane in any workspace: got %+v", p)
	}
	if got := st.WorkspaceLabel(); got != "backend" {
		t.Errorf("workspace label: got %q", got)
	}
}

func TestLoadWithoutHerdr(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	st := Load(t.TempDir())
	if st.Available {
		t.Fatal("expected herdr to be unavailable")
	}
	if st.Pane("x", "") != nil {
		t.Fatal("expected no panes")
	}
}
