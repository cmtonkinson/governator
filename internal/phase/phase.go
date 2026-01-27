// Package phase defines the planning phase sequence and artifact gates.
package phase

import (
	"fmt"
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

// String returns a human-friendly label for the phase.
func (p Phase) String() string {
	if int(p) < 0 || int(p) >= len(phaseNames) {
		return fmt.Sprintf("unknown(%d)", int(p))
	}
	return phaseNames[p]
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
