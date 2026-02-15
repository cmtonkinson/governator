// Package tui tests interactive status model behavior.
package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/cmtonkinson/governator/internal/config"
	"github.com/cmtonkinson/governator/internal/index"
	"github.com/cmtonkinson/governator/internal/status"
)

// TestReadSupervisorLogTailReturnsTrailingLines verifies fixed-size tail semantics.
func TestReadSupervisorLogTailReturnsTrailingLines(t *testing.T) {
	repoRoot := t.TempDir()
	logPath := filepath.Join(repoRoot, "_governator", "_local-state", "supervisor", "supervisor.log")
	if err := os.MkdirAll(filepath.Dir(logPath), 0o755); err != nil {
		t.Fatalf("mkdir supervisor dir: %v", err)
	}
	content := strings.Join([]string{"l1", "l2", "l3", "l4", "l5", "l6"}, "\n") + "\n"
	if err := os.WriteFile(logPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write supervisor log: %v", err)
	}

	lines, err := readSupervisorLogTail(repoRoot, []status.SupervisorSummary{{LogPath: logPath}}, 5)
	if err != nil {
		t.Fatalf("readSupervisorLogTail error: %v", err)
	}

	want := []string{"l2", "l3", "l4", "l5", "l6"}
	if len(lines) != len(want) {
		t.Fatalf("line count = %d, want %d", len(lines), len(want))
	}
	for i := range want {
		if lines[i] != want[i] {
			t.Fatalf("lines[%d] = %q, want %q", i, lines[i], want[i])
		}
	}
}

// TestRenderSupervisorLogTailViewportHeight ensures log panel always reserves viewport rows.
func TestRenderSupervisorLogTailViewportHeight(t *testing.T) {
	rendered := renderSupervisorLogTail([]string{"a", "b"}, 5)
	rows := strings.Split(rendered, "\n")
	if len(rows) != 5 {
		t.Fatalf("rendered rows = %d, want 5", len(rows))
	}
	if strings.TrimSpace(rows[3]) != "a" {
		t.Fatalf("row 4 = %q, want 'a'", rows[3])
	}
	if strings.TrimSpace(rows[4]) != "b" {
		t.Fatalf("row 5 = %q, want 'b'", rows[4])
	}
}

// TestViewUsesSharedStatusRenderer verifies TUI view renders shared status output.
func TestViewUsesSharedStatusRenderer(t *testing.T) {
	m := New(".")
	m.summary = status.Summary{}
	view := m.View()

	if !strings.Contains(view, "overall") {
		t.Fatalf("view missing shared status output: %q", view)
	}
}

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
	if !strings.Contains(view, "m: show merged") {
		t.Fatalf("view missing merged toggle help text: %q", view)
	}
}

// TestToggleMergedKey ensures interactive mode toggles merged visibility shortcut.
func TestToggleMergedKey(t *testing.T) {
	model := New(".")
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'m'}})
	next, ok := updated.(Model)
	if !ok {
		t.Fatalf("updated model type = %T, want tui.Model", updated)
	}
	if !next.showMerged {
		t.Fatal("expected showMerged to toggle true after pressing m")
	}
}

// TestRenderOnceReturnsSnapshot verifies one-shot rendering works without interactive mode.
func TestRenderOnceReturnsSnapshot(t *testing.T) {
	repoRoot := t.TempDir()
	if err := config.InitFullLayout(repoRoot, config.InitOptions{}); err != nil {
		t.Fatalf("init layout: %v", err)
	}
	indexPath := filepath.Join(repoRoot, "_governator", "_local-state", "index.json")
	if err := index.Save(indexPath, index.Index{SchemaVersion: 1}); err != nil {
		t.Fatalf("seed index: %v", err)
	}

	got, err := RenderOnce(repoRoot)
	if err != nil {
		t.Fatalf("RenderOnce error: %v", err)
	}
	if !strings.Contains(got, "overall") {
		t.Fatalf("snapshot missing overall section: %q", got)
	}
	if !strings.Contains(got, "Tasks") {
		if !strings.Contains(got, "tasks") {
			t.Fatalf("snapshot missing tasks section: %q", got)
		}
	}
}
