package run

import (
	"github.com/cmtonkinson/governator/internal/phase"
)

// stepToPhase maps a step name to its corresponding phase.Phase value.
// This is used for backward compatibility with older phase-based logic.
func stepToPhase(stepName string) phase.Phase {
	switch stepName {
	case "architecture-baseline":
		return phase.PhaseArchitectureBaseline
	case "gap-analysis":
		return phase.PhaseGapAnalysis
	case "project-planning":
		return phase.PhaseProjectPlanning
	case "task-planning":
		return phase.PhaseTaskPlanning
	default:
		return phase.PhaseNew
	}
}