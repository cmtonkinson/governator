// Package phase tracks Governator's project phase machine and stores durable metadata.
package phase

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

const (
	durableStateDir = "_governator/_durable_state"
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

// AgentMetadata captures the PID and timestamps for the agent that ran the phase.
type AgentMetadata struct {
	PID        int       `json:"pid,omitempty"`
	StartedAt  time.Time `json:"started_at,omitempty"`
	FinishedAt time.Time `json:"finished_at,omitempty"`
}

// PhaseRecord stores durable metadata for a specific phase.
type PhaseRecord struct {
	Agent       AgentMetadata `json:"agent,omitempty"`
	CompletedAt time.Time     `json:"completed_at,omitempty"`
}

// ArtifactValidation holds the result from validating a runnable phase gate.
type ArtifactValidation struct {
	Name      string    `json:"name"`
	Valid     bool      `json:"valid"`
	Message   string    `json:"message,omitempty"`
	CheckedAt time.Time `json:"checked_at"`
}

// State models the persisted phase tracker entry.
type State struct {
	Current             Phase                  `json:"current"`
	LastCompleted       Phase                  `json:"last_completed"`
	Records             map[string]PhaseRecord `json:"records"`
	ArtifactValidations []ArtifactValidation   `json:"artifact_validations,omitempty"`
	Notes               string                 `json:"notes,omitempty"`
}

// DefaultState returns the initial state written when no state file exists.
func DefaultState() State {
	return State{
		Current:       PhaseArchitectureBaseline,
		LastCompleted: PhaseNew,
		Records:       newPhaseRecords(),
	}
}

func newPhaseRecords() map[string]PhaseRecord {
	records := make(map[string]PhaseRecord, len(phaseNames))
	for _, name := range phaseNames {
		records[name] = PhaseRecord{}
	}
	return records
}

func mergePhaseRecords(current map[string]PhaseRecord) map[string]PhaseRecord {
	if len(current) == 0 {
		return newPhaseRecords()
	}
	records := make(map[string]PhaseRecord, len(phaseNames))
	for _, name := range phaseNames {
		if entry, ok := current[name]; ok {
			records[name] = entry
			continue
		}
		records[name] = PhaseRecord{}
	}
	return records
}

// RecordFor returns the stored phase record for p.
func (s *State) RecordFor(p Phase) PhaseRecord {
	if s.Records == nil {
		s.Records = newPhaseRecords()
	}
	record, ok := s.Records[p.String()]
	if !ok {
		record = PhaseRecord{}
		s.Records[p.String()] = record
	}
	return record
}

// SetRecord overwrites the stored phase record for p.
func (s *State) SetRecord(p Phase, record PhaseRecord) {
	if s.Records == nil {
		s.Records = newPhaseRecords()
	}
	s.Records[p.String()] = record
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
	if state.LastCompleted < PhaseNew || state.LastCompleted > PhaseComplete {
		state.LastCompleted = PhaseNew
	}
	if state.Records == nil {
		state.Records = newPhaseRecords()
	} else {
		state.Records = mergePhaseRecords(state.Records)
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
