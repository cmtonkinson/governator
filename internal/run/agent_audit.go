// Package run provides helper functions for audit logging agent events.
package run

import (
	"fmt"

	"github.com/cmtonkinson/governator/internal/index"
	"github.com/cmtonkinson/governator/internal/roles"
	"github.com/cmtonkinson/governator/internal/worker"
)

// AgentAuditor defines the audit methods needed for agent events.
type AgentAuditor interface {
	LogAgentInvoke(taskID string, role string, agent string, attempt int) error
	LogAgentOutcome(taskID string, role string, agent string, status string, exitCode int) error
}

// agentNameForStage maps a lifecycle stage to an audit agent name.
func agentNameForStage(stage roles.Stage) string {
	switch stage {
	case roles.StageWork:
		return "worker"
	case roles.StageTest:
		return "tester"
	case roles.StageReview:
		return "reviewer"
	case roles.StageResolve:
		return "resolver"
	default:
		return "agent"
	}
}

// logAgentInvoke emits an agent.invoke audit entry when a worker starts.
func logAgentInvoke(auditor AgentAuditor, taskID string, role index.Role, stage roles.Stage, attempt int, warn func(string)) {
	if auditor == nil {
		return
	}
	if taskID == "" {
		return
	}
	if role == "" {
		return
	}
	if attempt < 1 {
		attempt = 1
	}
	if err := auditor.LogAgentInvoke(taskID, string(role), agentNameForStage(stage), attempt); err != nil {
		if warn != nil {
			warn(fmt.Sprintf("failed to log agent invoke for %s: %v", taskID, err))
		}
	}
}

// logAgentOutcome emits an agent.outcome audit entry when a worker completes.
func logAgentOutcome(auditor AgentAuditor, taskID string, role index.Role, stage roles.Stage, status string, exitCode int, warn func(string)) {
	if auditor == nil {
		return
	}
	if taskID == "" {
		return
	}
	if role == "" {
		return
	}
	if status == "" {
		status = "unknown"
	}
	if err := auditor.LogAgentOutcome(taskID, string(role), agentNameForStage(stage), status, exitCode); err != nil {
		if warn != nil {
			warn(fmt.Sprintf("failed to log agent outcome for %s: %v", taskID, err))
		}
	}
}

// statusFromIngestResult derives an audit status for a worker outcome.
func statusFromIngestResult(result worker.IngestResult) string {
	if result.TimedOut {
		return "timeout"
	}
	if result.Success {
		return "success"
	}
	return "failed"
}

// exitCodeForOutcome selects the most relevant exit code for an agent outcome.
func exitCodeForOutcome(exitCode int, timedOut bool) int {
	if timedOut {
		return -1
	}
	return exitCode
}
