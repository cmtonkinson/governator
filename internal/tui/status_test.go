// Package tui tests interactive status model behavior.
package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

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

// TestViewIncludesSupervisorLogTailSection verifies status TUI includes log tail panel.
func TestViewIncludesSupervisorLogTailSection(t *testing.T) {
	m := New(".")
	m.supervisorTail = []string{"line one", "line two"}
	view := m.View()

	if !strings.Contains(view, "Supervisor Log Tail (5)") {
		t.Fatalf("view missing log tail title: %q", view)
	}
	if !strings.Contains(view, "line one") || !strings.Contains(view, "line two") {
		t.Fatalf("view missing tail lines: %q", view)
	}
}
