// Package phase defines the planning phase sequence and artifact gates.
package phase

import (
	"fmt"
	"strings"
)

// Phase represents each numbered Governator phase.
type Phase int

const (
	PhaseNew Phase = iota
	PhaseArchitectureBaseline
	PhaseGapAnalysis
	PhaseProjectPlanning
	PhaseTaskPlanning
	PhaseExecution
	PhaseComplete
)

var phaseNames = []string{
	"new",
	"architecture-baseline",
	"gap-analysis",
	"project-planning",
	"task-planning",
	"execution",
	"complete",
}

var phaseByName = map[string]Phase{
	"new":                   PhaseNew,
	"architecture-baseline": PhaseArchitectureBaseline,
	"gap-analysis":          PhaseGapAnalysis,
	"project-planning":      PhaseProjectPlanning,
	"task-planning":         PhaseTaskPlanning,
	"execution":             PhaseExecution,
	"complete":              PhaseComplete,
}

// String returns a human-friendly label for the phase.
func (p Phase) String() string {
	if int(p) < 0 || int(p) >= len(phaseNames) {
		return fmt.Sprintf("unknown(%d)", int(p))
	}
	return phaseNames[p]
}

// ParsePhase resolves a phase enum from its string name.
func ParsePhase(name string) (Phase, error) {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return PhaseNew, fmt.Errorf("phase name is required")
	}
	if phase, ok := phaseByName[trimmed]; ok {
		return phase, nil
	}
	return PhaseNew, fmt.Errorf("unknown phase %q", trimmed)
}

// Number returns the numeric representation of the phase.
func (p Phase) Number() int {
	return int(p)
}

// Next returns the phase that follows the provided one.
func (p Phase) Next() Phase {
	switch p {
	case PhaseNew:
		return PhaseArchitectureBaseline
	case PhaseArchitectureBaseline:
		return PhaseGapAnalysis
	case PhaseGapAnalysis:
		return PhaseProjectPlanning
	case PhaseProjectPlanning:
		return PhaseTaskPlanning
	case PhaseTaskPlanning:
		return PhaseExecution
	case PhaseExecution:
		return PhaseExecution
	case PhaseComplete:
		return PhaseComplete
	default:
		return PhaseArchitectureBaseline
	}
}
