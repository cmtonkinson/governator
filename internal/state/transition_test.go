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
		{TaskStateOpen, TaskStateWorked},
		{TaskStateOpen, TaskStateBlocked},
		{TaskStateWorked, TaskStateTested},
		{TaskStateWorked, TaskStateBlocked},
		{TaskStateTested, TaskStateDone},
		{TaskStateTested, TaskStateConflict},
		{TaskStateTested, TaskStateBlocked},
		{TaskStateConflict, TaskStateResolved},
		{TaskStateConflict, TaskStateBlocked},
		{TaskStateResolved, TaskStateDone},
		{TaskStateResolved, TaskStateConflict},
		{TaskStateBlocked, TaskStateOpen},
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
		{TaskStateDone, TaskStateWorked},
		{TaskStateBlocked, TaskStateDone},
		{TaskStateResolved, TaskStateWorked},
		{TaskStateOpen, TaskStateDone},
		{"", TaskStateOpen},
		{TaskStateOpen, ""},
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
