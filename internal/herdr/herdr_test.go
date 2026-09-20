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
	t.Setenv("HERDR_BIN_PATH", "") // the fake must win even when the tests run in a Herdr pane
}

func TestLoadMapsPanesToRepos(t *testing.T) {
	root := t.TempDir()
	fakeHerdr(t, root)
	t.Setenv("HERDR_WORKSPACE_ID", "w1")

	st, err := Load(root)
	if err != nil {
		t.Fatal(err)
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
	t.Setenv("HERDR_BIN_PATH", "")
	t.Setenv("HERDR_WORKSPACE_ID", "w")
	st, err := Load(t.TempDir())
	if err == nil {
		t.Fatal("expected an error without herdr")
	}
	if st.Pane("x", "") != nil || st.WorkspaceLabel() != "w" {
		t.Fatalf("expected an empty state for the workspace, got %+v", st)
	}
}

func TestBinaryPrefersHerdrBinPath(t *testing.T) {
	t.Setenv("HERDR_BIN_PATH", "/opt/herdr")
	if binary() != "/opt/herdr" {
		t.Fatal("HERDR_BIN_PATH ignored")
	}
	t.Setenv("HERDR_BIN_PATH", "")
	if binary() != "herdr" {
		t.Fatal("PATH lookup expected")
	}
}

func TestOpenCockpitFocusesExistingOrOpens(t *testing.T) {
	log := filepath.Join(t.TempDir(), "calls")
	t.Setenv("HERDR_CALLS", log)
	// A new cockpit opens unfocused and is then focused by tab and pane: an
	// explicit tab focus is what the Herdr client draws after a key binding.
	const opened = "plugin pane open --plugin x.y --entrypoint cockpit --workspace w1 --no-focus\ntab focus w1:tN\nplugin pane focus w1:pN\n"
	for _, tt := range []struct {
		name, panes, want string
	}{
		{"focus", `{"pane_id":"w1:p9","tab_id":"w1:t9","workspace_id":"w1","label":"lazyherd"}`, "tab focus w1:t9\nplugin pane focus w1:p9\n"},
		{"other workspace", `{"pane_id":"w2:p9","tab_id":"w2:t9","workspace_id":"w2","label":"lazyherd"}`, opened},
		{"other label", `{"pane_id":"w1:p9","tab_id":"w1:t9","workspace_id":"w1","label":"shell"}`, opened},
	} {
		t.Run(tt.name, func(t *testing.T) {
			scriptOnPath(t, "herdr", `case "$1 $2 $3" in
  "pane list ") printf '%s' '{"result":{"panes":[`+tt.panes+`]}}';;
  "plugin pane open") echo "$*" >> "$HERDR_CALLS"; printf '%s' '{"result":{"plugin_pane":{"pane":{"pane_id":"w1:pN","tab_id":"w1:tN"}}}}';;
  *) echo "$*" >> "$HERDR_CALLS";;
esac`)
			os.Remove(log)
			if err := OpenCockpit("x.y", "cockpit", "lazyherd", "w1"); err != nil {
				t.Fatal(err)
			}
			raw, err := os.ReadFile(log)
			if err != nil || string(raw) != tt.want {
				t.Fatalf("calls %q, %v", raw, err)
			}
		})
	}
	scriptOnPath(t, "herdr", `echo "no server" >&2; exit 1`)
	if err := OpenCockpit("x.y", "cockpit", "lazyherd", "w1"); err == nil || err.Error() != "herdr pane list: no server" {
		t.Fatalf("listing failure: %v", err)
	}
	scriptOnPath(t, "herdr", `case "$1 $2" in "pane list") printf '%s' '{"result":{"panes":[]}}';; *) printf '%s' '{"result":{}}';; esac`)
	if err := OpenCockpit("x.y", "cockpit", "lazyherd", "w1"); err == nil {
		t.Fatal("open without a pane id accepted")
	}
}
