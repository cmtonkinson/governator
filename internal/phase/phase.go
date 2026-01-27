// Package phase tracks Governator's project phase machine and stores durable metadata.
package phase

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

const (
	durableStateDir = "_governator/_durable-state"
	phaseStateFile  = "phase-state.json"
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

// State models the persisted phase tracker entry.
type State struct {
	Current Phase `json:"current"`
}

// DefaultState returns the initial state written when no state file exists.
func DefaultState() State {
	return State{
		Current: PhaseArchitectureBaseline,
	}
}

// Store provides durable persistence for phase metadata.
type Store struct {
	path string
}

// NewStore returns a store that reads and writes the shared phase state file.
func NewStore(repoRoot string) *Store {
	return &Store{
		path: filepath.Join(repoRoot, durableStateDir, phaseStateFile),
	}
}

// Path reports the configured state file path.
func (s *Store) Path() string {
	return s.path
}

// Load retrieves the current state, returning the default when no file exists.
func (s *Store) Load() (State, error) {
	var state State
	data, err := os.ReadFile(s.path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return DefaultState(), nil
		}
		return state, fmt.Errorf("read phase state: %w", err)
	}
	if err := json.Unmarshal(data, &state); err != nil {
		return state, fmt.Errorf("unmarshal phase state: %w", err)
	}
	if state.Current < PhaseNew || state.Current > PhaseComplete {
		state.Current = PhaseArchitectureBaseline
	}
	return state, nil
}

// Save persists the provided state to disk.
func (s *Store) Save(state State) error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return fmt.Errorf("create phase state directory: %w", err)
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal phase state: %w", err)
	}
	if err := os.WriteFile(s.path, data, 0o644); err != nil {
		return fmt.Errorf("write phase state: %w", err)
	}
	return nil
}
