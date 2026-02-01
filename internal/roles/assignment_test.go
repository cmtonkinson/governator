// Package roles provides tests for role assignment prompt building and selection.
package roles

import (
	"context"
	"strings"
	"testing"

	"github.com/cmtonkinson/governator/internal/index"
)

// TestSelectRoleHappyPath ensures a valid response selects the LLM role.
func TestSelectRoleHappyPath(t *testing.T) {
	t.Helper()
	request := sampleRoleAssignmentRequest()
	invoker := &stubInvoker{
		response: `{"role":"engineer","rationale":"Implementation fits the engineer role."}`,
	}
	auditor := &stubAuditLogger{}
	var warnings []string

	result, err := SelectRole(context.Background(), invoker, "prompt", request, func(message string) {
		warnings = append(warnings, message)
	}, auditor)
	if err != nil {
		t.Fatalf("SelectRole error: %v", err)
	}
	if result.Role != "engineer" {
		t.Fatalf("Role = %q, want %q", result.Role, "engineer")
	}
	if result.Fallback {
		t.Fatal("Fallback = true, want false")
	}
	if result.Rationale == "" {
		t.Fatal("Rationale is empty")
	}
	if len(warnings) != 0 {
		t.Fatalf("warnings = %v, want none", warnings)
	}
	if invoker.prompt == "" || !strings.Contains(invoker.prompt, "\"task\"") {
		t.Fatal("prompt should include request JSON payload")
	}
	if len(auditor.outcomes) != 1 {
		t.Fatalf("audit outcomes = %d, want 1", len(auditor.outcomes))
	}
	outcome := auditor.outcomes[0]
	if outcome.status != "selected" || outcome.exitCode != 0 {
		t.Fatalf("audit outcome = %#v, want status selected exit_code 0", outcome)
	}
	if outcome.role != "engineer" {
		t.Fatalf("audit role = %q, want %q", outcome.role, "engineer")
	}
}

// TestSelectRoleInvalidRoleFallback ensures invalid roles fall back deterministically.
func TestSelectRoleInvalidRoleFallback(t *testing.T) {
	t.Helper()
	request := sampleRoleAssignmentRequest()
	invoker := &stubInvoker{
		response: `{"role":"ghost","rationale":"No idea."}`,
	}
	auditor := &stubAuditLogger{}
	var warnings []string

	result, err := SelectRole(context.Background(), invoker, "prompt", request, func(message string) {
		warnings = append(warnings, message)
	}, auditor)
	if err != nil {
		t.Fatalf("SelectRole error: %v", err)
	}
	if result.Role != "architect" {
		t.Fatalf("Role = %q, want %q", result.Role, "architect")
	}
	if !result.Fallback {
		t.Fatal("Fallback = false, want true")
	}
	if len(warnings) != 1 {
		t.Fatalf("warnings = %v, want 1 warning", warnings)
	}
	if !strings.Contains(warnings[0], "invalid role assignment response") {
		t.Fatalf("warning = %q, want invalid role assignment response", warnings[0])
	}
	if len(auditor.outcomes) != 1 {
		t.Fatalf("audit outcomes = %d, want 1", len(auditor.outcomes))
	}
	outcome := auditor.outcomes[0]
	if outcome.status != "fallback" || outcome.exitCode != 1 {
		t.Fatalf("audit outcome = %#v, want status fallback exit_code 1", outcome)
	}
	if outcome.role != "architect" {
		t.Fatalf("audit role = %q, want %q", outcome.role, "architect")
	}
}

// TestSelectRoleInvalidJSONFallback ensures invalid JSON responses trigger fallback.
func TestSelectRoleInvalidJSONFallback(t *testing.T) {
	t.Helper()
	request := sampleRoleAssignmentRequest()
	invoker := &stubInvoker{
		response: `not-json`,
	}
	auditor := &stubAuditLogger{}
	var warnings []string

	result, err := SelectRole(context.Background(), invoker, "prompt", request, func(message string) {
		warnings = append(warnings, message)
	}, auditor)
	if err != nil {
		t.Fatalf("SelectRole error: %v", err)
	}
	if result.Role != "architect" {
		t.Fatalf("Role = %q, want %q", result.Role, "architect")
	}
	if !result.Fallback {
		t.Fatal("Fallback = false, want true")
	}
	if len(warnings) != 1 {
		t.Fatalf("warnings = %v, want 1 warning", warnings)
	}
}

// stubInvoker captures prompts and returns a canned response.
type stubInvoker struct {
	prompt   string
	response string
	err      error
}

// Invoke records the prompt and returns the configured response.
func (invoker *stubInvoker) Invoke(_ context.Context, prompt string) (string, error) {
	invoker.prompt = prompt
	return invoker.response, invoker.err
}

// stubAuditLogger records agent outcome events for assertions.
type stubAuditLogger struct {
	outcomes []auditOutcome
}

// LogAgentOutcome captures the agent outcome payload.
func (logger *stubAuditLogger) LogAgentOutcome(taskID string, role string, agent string, status string, exitCode int) error {
	logger.outcomes = append(logger.outcomes, auditOutcome{
		taskID:   taskID,
		role:     role,
		agent:    agent,
		status:   status,
		exitCode: exitCode,
	})
	return nil
}

// auditOutcome stores details of a logged agent outcome event.
type auditOutcome struct {
	taskID   string
	role     string
	agent    string
	status   string
	exitCode int
}

// sampleRoleAssignmentRequest builds a valid role assignment request fixture.
func sampleRoleAssignmentRequest() RoleAssignmentRequest {
	return RoleAssignmentRequest{
		Task: RoleAssignmentTask{
			ID:      "T-01",
			Title:   "Sample task",
			Path:    "_governator/tasks/task-01.md",
			Content: "# Task 01\n\nDo the thing.",
		},
		Stage:          StageWork,
		AvailableRoles: []index.Role{"architect", "engineer"},
		Caps: RoleAssignmentCaps{
			Global:      2,
			DefaultRole: 1,
			Roles: roleIntMap{
				"architect": 1,
				"engineer":  1,
			},
			InFlight: roleIntMap{
				"architect": 0,
				"engineer":  0,
			},
		},
	}
}
