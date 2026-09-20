package repo

import "testing"

func TestParseStatus(t *testing.T) {
	out := `# branch.oid 0123456789abcdef
# branch.head main
# branch.upstream origin/main
# branch.ab +2 -1
1 .M N... 100644 100644 100644 aaaa bbbb README.md
1 A. N... 000000 100644 100644 0000 cccc internal/new file.go
2 R. N... 100644 100644 100644 dddd dddd R100 new/name.go	old/name.go
u UU N... 100644 100644 100644 100644 eeee ffff gggg conflict.txt
? docs/shot.png
`
	st := parseStatus(out)
	if st.Head != "0123456789abcdef" || st.Branch != "main" || st.Ahead != 2 || st.Behind != 1 || st.NoUpstream {
		t.Fatalf("branch info: %+v", st)
	}
	want := []struct{ path, code string }{
		{"README.md", " M"}, {"internal/new file.go", "A "}, {"new/name.go", "R "}, {"conflict.txt", "UU"}, {"docs/shot.png", "??"},
	}
	if len(st.Changes) != len(want) {
		t.Fatalf("changes: %+v", st.Changes)
	}
	for i, w := range want {
		if st.Changes[i].Path != w.path || st.Changes[i].Code() != w.code {
			t.Errorf("change %d: got %q %q, want %q %q", i, st.Changes[i].Path, st.Changes[i].Code(), w.path, w.code)
		}
	}
}

func TestParseStatusDetachedNoUpstream(t *testing.T) {
	st := parseStatus("# branch.oid 0123456789abcdef\n# branch.head (detached)\n")
	if st.Branch != "@0123456" || !st.NoUpstream || len(st.Changes) != 0 {
		t.Fatalf("got %+v", st)
	}
}

func TestAgo(t *testing.T) {
	now := int64(1_000_000_000)
	cases := map[int64]string{0: "", now: "0s", now - 59: "59s", now - 60: "1m", now - 3599: "59m",
		now - 3600: "1h", now - 86400*3: "3d", now - 604800*2: "2w", now - 31536000/12*4: "4M", now - 31536000*2: "2y", now + 10: "0s"}
	for ts, want := range cases {
		if got := Ago(now, ts); got != want {
			t.Errorf("Ago(%d) = %q, want %q", ts, got, want)
		}
	}
}
