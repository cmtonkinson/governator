// Package run defines workstream abstractions used by the run orchestration.
package run

import (
	"fmt"

	"github.com/cmtonkinson/governator/internal/index"
	"github.com/cmtonkinson/governator/internal/phase"
)

// workstreamStepActions captures deterministic side effects that follow a successful step.
type workstreamStepActions struct {
	mergeToBase  bool
	advancePhase bool
}

// workstreamGateTarget declares which phase gate to evaluate at a transition point.
type workstreamGateTarget struct {
	enabled bool
	phase   phase.Phase
}

// workstreamStepGates groups the gate targets checked before dispatch and before advancing.
type workstreamStepGates struct {
	beforeDispatch workstreamGateTarget
	beforeAdvance  workstreamGateTarget
}

// workstreamStep is the minimal unit required to drive a phase-aware workstream.
type workstreamStep struct {
	phase      phase.Phase
	name       string
	promptPath string
	role       index.Role
	actions    workstreamStepActions
	gates      workstreamStepGates
}

// workstreamID returns the stable workstream identifier for the step.
func (step workstreamStep) workstreamID() string {
	return fmt.Sprintf("phase-%s", step.phase.String())
}

// branchName returns the branch name associated with the step workstream.
func (step workstreamStep) branchName() string {
	return step.workstreamID()
}

// title returns a human-friendly title used in commit messages.
func (step workstreamStep) title() string {
	return fmt.Sprintf("Phase %d %s", step.phase.Number(), step.phase.String())
}

// phaseWorkstream provides step lookup for a phase-aware workstream definition.
type phaseWorkstream interface {
	stepForPhase(phase.Phase) (workstreamStep, bool)
}
