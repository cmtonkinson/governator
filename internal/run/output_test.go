package run

import (
	"bytes"
	"testing"

	"github.com/cmtonkinson/governator/internal/roles"
)

func TestTaskEventFormatting(t *testing.T) {
	var buf bytes.Buffer
	emitTaskStart(&buf, "T-001", "tester", string(roles.StageTest))
	emitTaskComplete(&buf, "T-001", "tester", string(roles.StageTest))

	got := buf.String()
	want := "task=T-001 role=tester stage=test status=start"
	gotLines := splitLines(got)
	if len(gotLines) < 2 {
		t.Fatalf("got %d lines, want at least 2", len(gotLines))
	}
	if gotLines[0] != want {
		t.Fatalf("start line = %q, want %q", gotLines[0], want)
	}
	if gotLines[1] != "task=T-001 role=tester stage=test status=complete" {
		t.Fatalf("complete line = %q", gotLines[1])
	}
}

func TestTaskTimeoutFormatting(t *testing.T) {
	var buf bytes.Buffer
	emitTaskTimeout(&buf, "T-002", "tester", string(roles.StageReview), "execution timed out", 42)
	got := buf.String()
	want := "task=T-002 role=tester stage=review status=timeout reason=\"execution timed out\" timeout_seconds=42\n"
	if got != want {
		t.Fatalf("timeout event = %q, want %q", got, want)
	}
}

func TestPlanningDriftMessage(t *testing.T) {
	var buf bytes.Buffer
	emitPlanningDriftMessage(&buf, "Planning drift detected")
	got := buf.String()
	want := "planning=drift status=blocked reason=\"Planning drift detected\" next_step=\"governator start\"\n"
	if got != want {
		t.Fatalf("drift message = %q, want %q", got, want)
	}
}

func TestADRReplanMessage(t *testing.T) {
	var buf bytes.Buffer
	emitADRReplanMessage(&buf, "ADR drift detected")
	got := buf.String()
	want := "planning=drift status=drain reason=\"ADR drift detected\" next_step=\"auto-replan\"\n"
	if got != want {
		t.Fatalf("replan message = %q, want %q", got, want)
	}
}

func splitLines(s string) []string {
	var lines []string
	for _, line := range bytes.Split([]byte(s), []byte("\n")) {
		if len(line) == 0 {
			continue
		}
		lines = append(lines, string(line))
	}
	return lines
}
