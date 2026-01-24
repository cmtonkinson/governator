// Tests for lifecycle transition guards.
package state

import (
	"strings"
	"testing"
)

// TestValidTransitionsAcceptsAllowedPairs ensures the state machine allows known transitions.
func TestValidTransitionsAcceptsAllowedPairs(t *testing.T) {
	cases := []struct {
		from TaskState
		to   TaskState
	}{
		{TaskStateBacklog, TaskStateTriaged},
		{TaskStateTriaged, TaskStateImplemented},
		{TaskStateTriaged, TaskStateBlocked},
		{TaskStateImplemented, TaskStateTested},
		{TaskStateTested, TaskStateReviewed},
		{TaskStateTested, TaskStateConflict},
		{TaskStateTested, TaskStateTriaged},
		{TaskStateTested, TaskStateBlocked},
		{TaskStateReviewed, TaskStateMergeable},
		{TaskStateReviewed, TaskStateBlocked},
		{TaskStateMergeable, TaskStateMerged},
		{TaskStateMergeable, TaskStateConflict},
		{TaskStateConflict, TaskStateResolved},
		{TaskStateConflict, TaskStateBlocked},
		{TaskStateResolved, TaskStateMergeable},
		{TaskStateResolved, TaskStateConflict},
		{TaskStateBlocked, TaskStateTriaged},
	}

	for _, tc := range cases {
		if !IsValidTransition(tc.from, tc.to) {
			t.Fatalf("expected transition from %q to %q to be valid", tc.from, tc.to)
		}
		if err := ValidateTransition(tc.from, tc.to); err != nil {
			t.Fatalf("unexpected error for %q to %q: %v", tc.from, tc.to, err)
		}
	}
}

// TestInvalidTransitionsRejectsUnknownPairs ensures disallowed transitions fail with errors.
func TestInvalidTransitionsRejectsUnknownPairs(t *testing.T) {
	cases := []struct {
		from TaskState
		to   TaskState
	}{
		{TaskStateMerged, TaskStateImplemented},
		{TaskStateBlocked, TaskStateMerged},
		{TaskStateResolved, TaskStateImplemented},
		{TaskStateBacklog, TaskStateMerged},
		{"", TaskStateOpen},
		{TaskStateTriaged, ""},
	}

	for _, tc := range cases {
		if IsValidTransition(tc.from, tc.to) {
			t.Fatalf("expected transition from %q to %q to be invalid", tc.from, tc.to)
		}
		err := ValidateTransition(tc.from, tc.to)
		if err == nil {
			t.Fatalf("expected error for %q to %q", tc.from, tc.to)
		}
		if !strings.Contains(err.Error(), "invalid task state transition") {
			t.Fatalf("expected concise transition error, got %v", err)
		}
	}
}
