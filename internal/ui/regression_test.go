package ui

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/chriopter/lazyherd/internal/herdr"
	"github.com/chriopter/lazyherd/internal/repo"
)

type selectionBuffer struct {
	bytes.Buffer
	closed bool
	err    error
}

func (b *selectionBuffer) Close() error { b.closed = true; return nil }
func (b *selectionBuffer) Write(p []byte) (int, error) {
	if b.err != nil {
		return 0, b.err
	}
	return b.Buffer.Write(p)
}

func TestNavigationBindingsAndBounds(t *testing.T) {
	for _, tt := range []struct {
		name        string
		key         tea.KeyMsg
		start, want int
	}{
		{"j", key("j"), 0, 1}, {"k", key("k"), 2, 1}, {"g", key("g"), 2, 0}, {"G", key("G"), 0, 2},
		{"home", tea.KeyMsg{Type: tea.KeyHome}, 2, 0}, {"end", tea.KeyMsg{Type: tea.KeyEnd}, 0, 2},
		{"down", tea.KeyMsg{Type: tea.KeyDown}, 0, 1}, {"up", tea.KeyMsg{Type: tea.KeyUp}, 2, 1},
		{"lower bound", key("k"), 0, 0}, {"upper bound", key("j"), 2, 2},
	} {
		t.Run(tt.name, func(t *testing.T) {
			m := newTestModel(t)
			m.setRepos([]repo.Repo{{Name: "a"}, {Name: "b"}, {Name: "c"}})
			m.cursor = tt.start
			next, _ := m.Update(tt.key)
			if next.(Model).cursor != tt.want {
				t.Fatalf("cursor %d, want %d", next.(Model).cursor, tt.want)
			}
		})
	}
	m := newTestModel(t)
	for _, k := range []string{"j", "k", "g", "G", "p", "P", "enter", "t", "space", "w"} {
		m, cmd := press(t, m, k)
		if m.cursor != 0 || m.current() != nil || cmd != nil {
			t.Fatalf("empty list key %q: cursor=%d cmd=%v", k, m.cursor, cmd != nil)
		}
	}
}

func TestFilterAcceptClearAndQuitBindings(t *testing.T) {
	m := newTestModel(t)
	m.setRepos([]repo.Repo{{Name: "alpha"}, {Name: "beta"}})
	m, _ = press(t, m, "/", "beta", "enter")
	if m.filtering || m.filter != "beta" || len(m.visible) != 1 || m.current().Name != "beta" {
		t.Fatalf("accepted filter: %+v", m)
	}
	m, cmd := press(t, m, "esc")
	if cmd != nil || m.filter != "" || len(m.visible) != 2 {
		t.Fatal("escape should clear accepted filter")
	}
	for _, k := range []tea.KeyMsg{key("q"), key("esc"), {Type: tea.KeyCtrlC}} {
		b := &selectionBuffer{}
		m.companion = b
		_, cmd := m.Update(k)
		if !b.closed || cmd == nil {
			t.Fatal("quit must close companion")
		}
		if _, ok := cmd().(tea.QuitMsg); !ok {
			t.Fatalf("key %v did not quit", k)
		}
	}
}

func TestStaleMessagesLeaveStateUntouched(t *testing.T) {
	for _, kind := range []string{"scan", "herdr", "selection"} {
		t.Run(kind, func(t *testing.T) {
			m := newTestModel(t)
			m.setRepos([]repo.Repo{{Name: "keep"}})
			m.scanning = true
			m.selSeq = 7
			b := &selectionBuffer{}
			m.companion = b
			m.herdr = herdr.State{Workspace: "keep"}
			var msg tea.Msg
			switch kind {
			case "scan":
				msg = scanMsg{gen: m.gen - 1, repos: []repo.Repo{{Name: "old"}}, err: errors.New("old")}
			case "herdr":
				msg = herdrMsg{gen: m.gen - 1, state: herdr.State{Workspace: "old"}}
			default:
				msg = selectionMsg(6)
			}
			next, cmd := m.Update(msg)
			got := next.(Model)
			if cmd != nil || got.current().Name != "keep" || got.herdr.Workspace != "keep" || !got.scanning || !got.loading || got.status != "" || b.Len() != 0 {
				t.Fatalf("stale %s applied: %+v", kind, got)
			}
		})
	}
}

func TestRefreshAndFetchWaitForAllBusyStates(t *testing.T) {
	for _, state := range []string{"sync", "fetch", "load", "scan"} {
		for _, event := range []string{"refresh", "fetch"} {
			t.Run(state+"/"+event, func(t *testing.T) {
				m := newTestModel(t)
				m.loading = false
				switch state {
				case "sync":
					m.busy = "syncing"
				case "fetch":
					m.fetching = true
				case "load":
					m.loading = true
				case "scan":
					m.scanning = true
				}
				var msg tea.Msg = refreshMsg{}
				if event == "fetch" {
					msg = autoFetchMsg{}
				}
				next, cmd := m.Update(msg)
				got := next.(Model)
				if got.scanning != m.scanning || got.fetching != m.fetching || got.gen != m.gen || cmd == nil {
					t.Fatalf("busy state changed: %+v", got)
				}
				// Do not execute the returned timer: it deliberately waits 3 or 60 seconds.
			})
		}
	}
	m := newTestModel(t)
	m.loading = false
	next, cmd := m.Update(refreshMsg{})
	if !next.(Model).scanning || cmd == nil {
		t.Fatal("idle refresh did not scan")
	}
	next, cmd = m.Update(autoFetchMsg{})
	if !next.(Model).fetching || cmd == nil {
		t.Fatal("idle fetch did not start")
	}
	m.scanning = true
	if m.scanCmds() != nil {
		t.Fatal("duplicate scan started")
	}
}

func TestSpinnerAdvancesOnlyWhileWorking(t *testing.T) {
	m := newTestModel(t)
	m.spin = 3
	next, cmd := m.Update(spinMsg{})
	m = next.(Model)
	if m.spin != 4 || cmd == nil {
		t.Fatal("busy spinner did not advance and rearm")
	}
	m.loading = false
	next, cmd = m.Update(spinMsg{})
	if next.(Model).spin != 4 || cmd != nil {
		t.Fatal("idle spinner continued")
	}
}

func TestSyncSummaryFormatting(t *testing.T) {
	for _, tt := range []struct {
		name    string
		results []repo.SyncResult
		want    string
	}{
		{"empty", nil, "sync: 0 pulled, 0 pushed"},
		{"mixed", []repo.SyncResult{{Name: "a", Pulled: true, Pushed: true}, {Name: "b", Pushed: true}, {Name: "c"}}, "sync: 1 pulled, 2 pushed"},
		{"errors", []repo.SyncResult{{Name: "a", Pulled: true, Err: errors.New("push rejected")}, {Name: "b", Err: errors.New("pull failed")}}, "sync: 1 pulled, 0 pushed, failed: a (push rejected), b (pull failed)"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := syncSummary(tt.results); got != tt.want {
				t.Fatalf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestMouseRowMappingAndIgnoredEvents(t *testing.T) {
	m := newTestModel(t)
	m.width = 60
	m.height = 8
	m.setRepos([]repo.Repo{{Name: "a"}, {Name: "b"}, {Name: "c"}, {Name: "d"}})
	m.cursor = 3
	for _, tt := range []struct {
		y, want int
		ok      bool
	}{{-1, 0, false}, {0, 0, false}, {3, 0, false}, {4, 2, true}, {5, 3, true}, {6, 0, false}, {7, 0, false}, {100, 0, false}} {
		i, ok := m.repoRowAt(tt.y)
		if i != tt.want || ok != tt.ok {
			t.Errorf("row %d: %d %v", tt.y, i, ok)
		}
		next, _ := m.Update(tea.MouseMsg{X: 2, Y: tt.y, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft})
		want := 3
		if tt.ok {
			want = tt.want
		}
		if next.(Model).cursor != want {
			t.Errorf("row %d selected %d", tt.y, next.(Model).cursor)
		}
	}
	for _, msg := range []tea.MouseMsg{{X: 2, Y: 4, Action: tea.MouseActionRelease, Button: tea.MouseButtonLeft}, {X: 2, Y: 4, Action: tea.MouseActionPress, Button: tea.MouseButtonRight}, {X: 2, Y: 4, Action: tea.MouseActionMotion, Button: tea.MouseButtonLeft}} {
		next, _ := m.Update(msg)
		if next.(Model).cursor != 3 {
			t.Fatal("non-left-press moved selection")
		}
	}
	m.setRepos([]repo.Repo{{Name: "a"}})
	if _, ok := m.repoRowAt(5); ok {
		t.Fatal("blank row matched")
	}
}

func uiHerdrLog(t *testing.T) string {
	t.Helper()
	bin := t.TempDir()
	log := filepath.Join(bin, "calls")
	t.Setenv("UI_HERDR_LOG", log)
	if err := os.WriteFile(filepath.Join(bin, "herdr"), []byte("#!/bin/sh\nprintf '%s\\n' \"$@\" >> \"$UI_HERDR_LOG\"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	return log
}

func TestEnterWithAndWithoutCompanion(t *testing.T) {
	for _, k := range []tea.KeyMsg{key("enter"), key("l"), {Type: tea.KeyRight}} {
		t.Run(k.String(), func(t *testing.T) {
			m := newTestModel(t)
			m.setRepos([]repo.Repo{{Name: "api"}})
			_, cmd := m.Update(k)
			if cmd == nil {
				t.Fatal("standalone open returned no command")
			}
			// Bubble Tea owns execution of this opaque exec message.
			if msg := cmd(); fmt.Sprintf("%T", msg) != "tea.execMsg" {
				t.Fatalf("expected exec message, got %T", msg)
			}
			log := uiHerdrLog(t)
			b := &selectionBuffer{}
			m.companion = b
			m.companionPane = "child"
			m.ownPane = "own"
			next, cmd := m.Update(k)
			m = next.(Model)
			if m.shown != "api" || b.String() != filepath.Join(m.root, "api")+"\n" || cmd == nil {
				t.Fatal("companion did not receive selection")
			}
			cmd()
			raw, err := os.ReadFile(log)
			if err != nil {
				t.Fatal(err)
			}
			if string(raw) != "pane\nfocus\n--direction\nright\n--pane\nown\n" {
				t.Fatalf("focus args: %q", raw)
			}
			m.showCurrent()
			if strings.Count(b.String(), "\n") != 1 {
				t.Fatal("same selection resent")
			}
		})
	}
}

func TestTabBindingFocusesOrCreates(t *testing.T) {
	for _, existing := range []bool{false, true} {
		t.Run(fmt.Sprint(existing), func(t *testing.T) {
			m := newTestModel(t)
			t.Setenv("HERDR_WORKSPACE_ID", "w")
			m.setRepos([]repo.Repo{{Name: "api"}})
			names := []string{}
			if existing {
				names = append(names, "api")
			}
			m.herdr = herdrStateWith(t, m.root, "w", names...)
			log := uiHerdrLog(t)
			_, cmd := press(t, m, "t")
			if cmd == nil {
				t.Fatal("missing tab command")
			}
			msg := cmd()
			want := "tab\ncreate\n--cwd\n" + filepath.Join(m.root, "api") + "\n--label\napi\n--focus\n--workspace\nw\n"
			if existing {
				want = "tab\nfocus\ntapi\n"
				if msg != statusMsg("→ api") {
					t.Fatalf("focus message: %+v", msg)
				}
			} else if msg != (tabCreatedMsg{name: "api"}) {
				t.Fatalf("create message: %+v", msg)
			}
			raw, err := os.ReadFile(log)
			if err != nil || string(raw) != want {
				t.Fatalf("calls %q, %v", raw, err)
			}
		})
	}
	m := newTestModel(t)
	m.setRepos([]repo.Repo{{Name: "a"}})
	if _, cmd := press(t, m, "t"); cmd != nil {
		t.Fatal("tab opened without herdr")
	}
}

func TestWorkspaceOrderAndPinPersistenceThroughModel(t *testing.T) {
	m := newTestModel(t)
	t.Setenv("HERDR_WORKSPACE_ID", "w")
	m.herdr = herdrStateWith(t, m.root, "w", "d", "b")
	m.setRepos([]repo.Repo{{Name: "a"}, {Name: "b"}, {Name: "c"}, {Name: "d"}, {Name: "e"}})
	names := func(m Model) []string {
		var out []string
		for _, r := range m.visibleRepos() {
			out = append(out, r.Name)
		}
		return out
	}
	if got := names(m); !reflect.DeepEqual(got, []string{"b", "d", "a", "c", "e"}) {
		t.Fatalf("unstable grouping: %v", got)
	}
	m.cursor = 3
	m, _ = press(t, m, "space")
	loaded, err := loadPins(m.pins.path)
	if err != nil || !loaded.has("w", "c") {
		t.Fatalf("pin not persisted: %+v %v", loaded, err)
	}
	if got := names(m); !reflect.DeepEqual(got, []string{"b", "c", "d", "a", "e"}) || m.current().Name != "c" {
		t.Fatalf("pin order/selection: %v", got)
	}
	m, _ = press(t, m, "w")
	if got := names(m); !reflect.DeepEqual(got, []string{"b", "c", "d"}) {
		t.Fatal(got)
	}
	m, _ = press(t, m, "space")
	loaded, err = loadPins(m.pins.path)
	if err != nil || loaded.has("w", "c") {
		t.Fatal("unpin not persisted")
	}
	next, _ := m.Update(herdrMsg{gen: m.gen, state: herdr.State{}})
	if next.(Model).workspaceOnly || len(next.(Model).visible) != 5 {
		t.Fatal("lost herdr did not restore all repos")
	}
}

func TestMessageErrorsAndRecovery(t *testing.T) {
	m := newTestModel(t)
	m.setRepos([]repo.Repo{{Name: "a"}})
	for _, tt := range []struct {
		msg  tea.Msg
		want string
	}{
		{scanMsg{gen: m.gen, err: errors.New("denied")}, "scan failed: denied"},
		{companionMsg{err: errors.New("split failed")}, "no companion pane: split failed"},
		{fetchDoneMsg{failed: []string{"a", "b"}}, "fetch failed for a, b"},
		{lazygitDoneMsg{err: errors.New("missing")}, "lazygit: missing"},
		{statusMsg("note"), "note"},
	} {
		next, _ := m.Update(tt.msg)
		if next.(Model).status != tt.want {
			t.Errorf("%T: %q", tt.msg, next.(Model).status)
		}
	}
	b := &selectionBuffer{err: errors.New("closed")}
	m.companion = b
	m.showCurrent()
	if m.companion != nil || m.status != "companion: closed" || m.shown != "" {
		t.Fatal("failed companion not disabled")
	}
}

func TestFrameBordersAndPlacement(t *testing.T) {
	for _, tt := range []struct{ name, runes string }{{"rounded", "─│╭╮╰╯"}, {"single", "─│┌┐└┘"}, {"double", "═║╔╗╚╝"}, {"bold", "━┃┏┓┗┛"}, {"hidden", "      "}} {
		t.Run(tt.name, func(t *testing.T) {
			m := newTestModel(t)
			m.theme.frame = frameRunes(tt.name)
			r := []rune(tt.runes)
			lines := strings.Split(m.frame(1, "Repos", "workspace", "2 of 3", []string{"body"}, 40, 4, lipgloss.NewStyle()), "\n")
			if len(lines) != 4 {
				t.Fatal(lines)
			}
			wantTop := string(r[2]) + string(r[0]) + "[1]" + string(r[0]) + "Repos" + strings.Repeat(string(r[0]), 15) + "workspace" + strings.Repeat(string(r[0]), 4) + string(r[3])
			wantBottom := string(r[4]) + strings.Repeat(string(r[0]), 31) + "2 of 3" + string(r[0]) + string(r[5])
			if lines[0] != wantTop || lines[3] != wantBottom || lines[1] != string(r[1])+"body"+strings.Repeat(" ", 34)+string(r[1]) || lines[2] != string(r[1])+strings.Repeat(" ", 38)+string(r[1]) {
				t.Fatalf("frame:\n%s\nwant top %q bottom %q", strings.Join(lines, "\n"), wantTop, wantBottom)
			}
		})
	}
}

func TestStatusPanelContent(t *testing.T) {
	for _, tt := range []struct {
		name string
		r    repo.Repo
		want string
	}{
		{"clean", repo.Repo{Name: "api", Status: repo.Status{Branch: "main"}}, "✓ api → main"},
		{"ahead", repo.Repo{Name: "api", Status: repo.Status{Branch: "topic", Ahead: 2}}, "↑2 api → topic"},
		{"behind", repo.Repo{Name: "api", Status: repo.Status{Branch: "main", Behind: 3}}, "↓3 api → main"},
		{"diverged", repo.Repo{Name: "api", Status: repo.Status{Branch: "main", Ahead: 2, Behind: 3}}, "↓3↑2 api → main"},
		{"no upstream", repo.Repo{Name: "api", Status: repo.Status{Branch: "main", NoUpstream: true}}, "api → main"},
		{"error", repo.Repo{Name: "api", Err: errors.New("bad")}, "! api → "},
	} {
		t.Run(tt.name, func(t *testing.T) {
			m := newTestModel(t)
			m.width = 60
			m.setRepos([]repo.Repo{tt.r})
			if got := m.statusPanel(); !strings.Contains(got, tt.want) {
				t.Fatalf("panel %q", got)
			}
		})
	}
	m := newTestModel(t)
	m.width = 60
	if !strings.Contains(m.statusPanel(), "scanning "+spinner[0]) {
		t.Fatal("missing loading spinner")
	}
	m.loading = false
	if strings.Contains(m.statusPanel(), "scanning") {
		t.Fatal("idle empty panel says scanning")
	}
}

func TestOptionsAndBottomLine(t *testing.T) {
	m := newTestModel(t)
	m.width = 24
	if got := m.options(); got != "Open: <enter> | …" {
		t.Fatalf("narrow options %q", got)
	}
	m.width = 200
	m.version = "1.2.3"
	if got := m.bottomLine(); !strings.Contains(got, "Sync listed: P | Filter: / | Quit: q") || !strings.HasSuffix(got, "lazyherd 1.2.3") || lipgloss.Width(got) != 200 {
		t.Fatalf("options %q", got)
	}
	m.status = "done"
	if !strings.Contains(m.bottomLine(), "done") || strings.Contains(m.bottomLine(), "Open:") {
		t.Fatal("status did not replace options")
	}
	m.filtering = true
	m.filter = "abc"
	if !strings.Contains(m.bottomLine(), "Filter: abc") {
		t.Fatal("filter prompt missing")
	}
	m.busy = "syncing api"
	if !strings.HasSuffix(m.bottomLine(), "syncing api "+spinner[0]) {
		t.Fatal("busy indicator missing")
	}
	m.busy = ""
	m.fetching = true
	if !strings.HasSuffix(m.bottomLine(), "fetching "+spinner[0]) {
		t.Fatal("fetch indicator missing")
	}
}

func TestViewSupportedSizesStayWithinWidth(t *testing.T) {
	m := newTestModel(t)
	for _, name := range []string{"empty", "loading", "repos", "filter", "workspace"} {
		t.Run(name, func(t *testing.T) {
			m.loading = name == "loading"
			if name == "repos" || name == "filter" || name == "workspace" {
				m.setRepos([]repo.Repo{{Name: strings.Repeat("日本語é", 20), Status: repo.Status{Branch: strings.Repeat("branch", 30), Ahead: 99, Behind: 99}}, {Name: "api"}})
			}
			if name == "filter" {
				m.filter = "日本"
				m.filtering = true
				m.refilter()
			}
			if name == "workspace" {
				m.filter = ""
				m.filtering = false
				m.herdr = herdr.State{Available: true, Workspace: strings.Repeat("long", 50)}
				m.refilter()
			}
			for w := 24; w <= 120; w++ {
				for _, h := range []int{8, 9, 12, 30} {
					m.width = w
					m.height = h
					view := m.View()
					lines := strings.Split(view, "\n")
					if len(lines) != h {
						t.Fatalf("%dx%d: %d lines", w, h, len(lines))
					}
					for i, line := range lines {
						if width := lipgloss.Width(line); width > w {
							t.Fatalf("%dx%d row %d width %d: %q", w, h, i, width, line)
						}
					}
				}
			}
		})
	}
}

func TestViewSmallSizesNeverPanics(t *testing.T) {
	m := newTestModel(t)
	for w := 0; w < 24; w++ {
		for h := 0; h < 10; h++ {
			m.width = w
			m.height = h
			got := m.View()
			if w == 0 && got != "" {
				t.Fatalf("zero width: %q", got)
			}
			if w > 0 && !strings.HasPrefix(got, "lazyherd needs"[:min(w, 14)]) {
				t.Fatalf("small screen: %q", got)
			}
		}
	}
}

func TestSyncBindingsUseOnlyRequestedRepos(t *testing.T) {
	for _, k := range []string{"p", "P"} {
		t.Run(k, func(t *testing.T) {
			m := newTestModel(t)
			m.setRepos([]repo.Repo{{Name: "api", Status: repo.Status{NoUpstream: true}}, {Name: "app", Status: repo.Status{NoUpstream: true}}, {Name: "web", Status: repo.Status{NoUpstream: true}}})
			m.filter = "ap"
			m.refilter()
			m.cursor = 1
			m, cmd := press(t, m, k)
			if cmd == nil {
				t.Fatal("missing sync command")
			}
			batch, ok := cmd().(tea.BatchMsg)
			if !ok || len(batch) != 2 {
				t.Fatal("expected sync and spinner commands")
			}
			result, ok := batch[0]().(syncDoneMsg)
			if !ok {
				t.Fatal("missing sync result")
			}
			want := syncDoneMsg{{Name: "app"}}
			if k == "P" {
				want = syncDoneMsg{{Name: "api"}, {Name: "app"}}
			}
			if !reflect.DeepEqual(result, want) {
				t.Fatalf("synced %+v, want %+v", result, want)
			}
			for _, again := range []string{"p", "P"} {
				if _, cmd := press(t, m, again); cmd != nil {
					t.Fatal("busy sync was started again")
				}
			}
		})
	}
}

func TestWorkspaceBindingsWithoutWorkspaceDoNothing(t *testing.T) {
	for _, state := range []herdr.State{{}, {Available: true}, {Workspace: "w"}} {
		m := newTestModel(t)
		m.herdr = state
		m.setRepos([]repo.Repo{{Name: "api"}})
		m, _ = press(t, m, "w")
		if m.workspaceOnly || len(m.visible) != 1 {
			t.Fatal("workspace filter without available workspace")
		}
		if state.Workspace == "" {
			m, _ = press(t, m, "space")
			if len(m.pins.byWorkspace) != 0 {
				t.Fatal("pin created without workspace")
			}
		}
	}
}

func TestQuitClosesCompanionPane(t *testing.T) {
	m := newTestModel(t)
	b := &selectionBuffer{}
	m.companion = b
	m.companionPane = "child"
	log := uiHerdrLog(t)
	_, cmd := press(t, m, "q")
	if !b.closed || cmd == nil {
		t.Fatal("quit did not close socket")
	}
	// Sequence's message is an unexported slice of tea.Cmd. Reflection lets the
	// test run those public commands in order without starting an actual terminal.
	sequence := reflect.ValueOf(cmd())
	if sequence.Kind() != reflect.Slice || sequence.Len() != 2 {
		t.Fatalf("unexpected quit sequence: %v", sequence)
	}
	first, ok := sequence.Index(0).Interface().(tea.Cmd)
	if !ok {
		t.Fatal("missing close command")
	}
	first()
	raw, err := os.ReadFile(log)
	if err != nil || string(raw) != "pane\nclose\nchild\n" {
		t.Fatalf("close call: %q %v", raw, err)
	}
	last, ok := sequence.Index(1).Interface().(tea.Cmd)
	if !ok {
		t.Fatal("missing quit command")
	}
	if _, ok := last().(tea.QuitMsg); !ok {
		t.Fatal("sequence did not end in quit")
	}
}

func TestFrameTruncatesBodyAndOmitsOversizedLabels(t *testing.T) {
	m := newTestModel(t)
	lines := strings.Split(m.frame(1, "Repos", strings.Repeat("subtitle", 10), strings.Repeat("footer", 10), []string{strings.Repeat("界", 30)}, 24, 3, lipgloss.NewStyle()), "\n")
	if strings.Contains(lines[0], "subtitle") || strings.Contains(lines[2], "footer") {
		t.Fatal("oversized labels displayed")
	}
	for _, line := range lines {
		if lipgloss.Width(line) != 24 {
			t.Fatalf("frame width: %d", lipgloss.Width(line))
		}
	}
}

func TestThemeSelectionAndSearchingBorders(t *testing.T) {
	m := newTestModel(t)
	if !reflect.DeepEqual(m.border(true), m.theme.activeBorder) || !reflect.DeepEqual(m.border(false), m.theme.inactiveBorder) {
		t.Fatal("incorrect active/inactive border")
	}
	m.filtering = true
	if !reflect.DeepEqual(m.border(true), m.theme.searchingBorder) || !reflect.DeepEqual(m.border(false), m.theme.inactiveBorder) {
		t.Fatal("incorrect search border")
	}
	selected := m.style(red, true)
	if !selected.GetBold() || selected.GetBackground() != m.theme.selectedBg.GetBackground() || selected.GetForeground() != red.GetForeground() {
		t.Fatal("selection lost theme colors")
	}
	for _, version := range []string{"2", "3"} {
		m.theme.icons = configIcons(version)
		if got := m.branch(repo.Repo{Status: repo.Status{Branch: "main"}}); got != m.theme.icons.branch+" main" {
			t.Fatal(got)
		}
		if got := m.branch(repo.Repo{Status: repo.Status{Branch: "@abcdefg"}}); got != m.theme.icons.detachedHead+" @abcdefg" {
			t.Fatal(got)
		}
	}
}
