// Package tui tests interactive status model behavior.
package tui

import (
	"strings"
	"testing"
	"time"
)

// TestNewTaskTableNotFocused ensures task rows are not interactively selectable.
func TestNewTaskTableNotFocused(t *testing.T) {
	model := New(".")
	if model.table.Focused() {
		t.Fatal("task table is focused; expected unfocused to disable row selector")
	}
}

// TestViewHelpDoesNotAdvertiseNavigation ensures footer help matches supported controls.
func TestViewHelpDoesNotAdvertiseNavigation(t *testing.T) {
	model := New(".")
	model.lastUpdate = time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)

	view := model.View()
	if strings.Contains(view, "navigate") || strings.Contains(view, "↑/↓") {
		t.Fatalf("view contains row-navigation help text: %q", view)
	}
	if !strings.Contains(view, "r: refresh") {
		t.Fatalf("view missing refresh help text: %q", view)
	}
}
