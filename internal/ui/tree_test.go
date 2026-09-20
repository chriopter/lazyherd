package ui

import (
	"testing"

	"github.com/chriopter/lazyherd/internal/repo"
)

func TestBuildTree(t *testing.T) {
	rows := buildTree([]repo.Change{
		{Path: "internal/ui/view.go", Unstaged: 'M', Staged: ' '},
		{Path: "README.md", Unstaged: 'M', Staged: ' '},
		{Path: "internal/repo/repo.go", Staged: 'A', Unstaged: ' '},
		{Path: "docs/new.png", Untracked: true},
	})
	want := []string{"README.md", "docs/", "new.png", "internal/", "repo/", "repo.go", "ui/", "view.go"}
	if len(rows) != len(want) {
		t.Fatalf("rows: got %d, want %d: %+v", len(rows), len(want), rows)
	}
	for i, w := range want {
		if rows[i].name != w {
			t.Errorf("row %d: got %q, want %q", i, rows[i].name, w)
		}
	}
	if rows[4].depth != 1 || rows[5].depth != 2 || rows[0].depth != 0 {
		t.Errorf("depths wrong: %+v", rows)
	}
	if files := fileRows(rows); len(files) != 4 || files[0] != 0 || files[1] != 2 {
		t.Errorf("file rows: %v", files)
	}
}
