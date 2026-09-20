package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
)

// Regressions for rendering bugs found by review.
func TestKnownBugSmallViewExceedsWidth(t *testing.T) {
	m := newTestModel(t)
	m.width = 10
	m.height = 3
	for _, line := range strings.Split(m.View(), "\n") {
		if lipgloss.Width(line) > m.width {
			t.Fatalf("line exceeds width %d: %q", m.width, line)
		}
	}
}

func TestKnownBugOptionsEllipsisGetsClipped(t *testing.T) {
	m := newTestModel(t)
	m.width = 25
	m.height = 8
	lines := strings.Split(m.View(), "\n")
	if !strings.Contains(lines[len(lines)-1], "…") {
		t.Fatalf("ellipsis clipped: %q", lines[len(lines)-1])
	}
}
