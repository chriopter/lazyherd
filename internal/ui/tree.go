package ui

import (
	"sort"
	"strings"

	"github.com/chriopter/lazyherd/internal/repo"
)

// treeRow is one line of the change tree: a directory or a changed file.
type treeRow struct {
	depth  int
	name   string
	change *repo.Change // nil for directories
}

// buildTree lays changed files out as an indented directory tree. Directories
// are always expanded; files keep their git status.
func buildTree(changes []repo.Change) []treeRow {
	sorted := append([]repo.Change(nil), changes...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Path < sorted[j].Path })

	var rows []treeRow
	var open []string // directory components currently on the stack
	for i := range sorted {
		parts := strings.Split(sorted[i].Path, "/")
		dirs, file := parts[:len(parts)-1], parts[len(parts)-1]
		common := 0
		for common < len(open) && common < len(dirs) && open[common] == dirs[common] {
			common++
		}
		open = open[:common]
		for _, d := range dirs[common:] {
			rows = append(rows, treeRow{depth: len(open), name: d + "/"})
			open = append(open, d)
		}
		rows = append(rows, treeRow{depth: len(dirs), name: file, change: &sorted[i]})
	}
	return rows
}

// fileRows returns the indices of the rows that are files.
func fileRows(rows []treeRow) []int {
	var idx []int
	for i, r := range rows {
		if r.change != nil {
			idx = append(idx, i)
		}
	}
	return idx
}
