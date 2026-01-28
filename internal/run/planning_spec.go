// Package run provides the planning spec loader and validation helpers.
package run

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/cmtonkinson/governator/internal/index"
	"github.com/cmtonkinson/governator/internal/phase"
)

const (
	planningSpecFilePath = "_governator/planning.json"
	planningSpecVersion  = 1
)

// PlanningSpec defines the JSON schema for the planning workstream.
type PlanningSpec struct {
	Version int                `json:"version"`
	Steps   []PlanningStepSpec `json:"steps"`
}

// PlanningStepSpec declares a single step in the planning workstream.
type PlanningStepSpec struct {
	ID     string            `json:"id"`
	Name   string            `json:"name"`
	Prompt string            `json:"prompt"`
	Role   string            `json:"role"`
	Gates  *PlanningGateSpec `json:"gates"`
}

// PlanningGateSpec captures the phase gates to check around a planning step.
type PlanningGateSpec struct {
	BeforeDispatch *string `json:"before_dispatch"`
	BeforeAdvance  *string `json:"before_advance"`
}

// LoadPlanningSpec reads and parses the planning spec from the repository.
func LoadPlanningSpec(repoRoot string) (PlanningSpec, error) {
	if strings.TrimSpace(repoRoot) == "" {
		return PlanningSpec{}, fmt.Errorf("repo root is required")
	}
	path := filepath.Join(repoRoot, planningSpecFilePath)
	data, err := os.ReadFile(path)
	if err != nil {
		return PlanningSpec{}, fmt.Errorf("read planning spec %s: %w", path, err)
	}
	spec, err := ParsePlanningSpec(data)
	if err != nil {
		return PlanningSpec{}, fmt.Errorf("parse planning spec %s: %w", path, err)
	}
	return spec, nil
}

// ParsePlanningSpec decodes and validates a planning spec payload.
func ParsePlanningSpec(data []byte) (PlanningSpec, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var spec PlanningSpec
	if err := decoder.Decode(&spec); err != nil {
		return PlanningSpec{}, fmt.Errorf("decode planning spec: %w", err)
	}
	if err := ensureNoTrailingJSON(decoder); err != nil {
		return PlanningSpec{}, err
	}
	if err := validatePlanningSpec(spec); err != nil {
		return PlanningSpec{}, err
	}
	return spec, nil
}

// ensureNoTrailingJSON rejects extra tokens after a parsed JSON object.
func ensureNoTrailingJSON(decoder *json.Decoder) error {
	var trailer any
	if err := decoder.Decode(&trailer); err != nil {
		if err == io.EOF {
			return nil
		}
		return fmt.Errorf("decode trailing data: %w", err)
	}
	return fmt.Errorf("unexpected trailing data")
}

// validatePlanningSpec enforces required fields and value constraints.
func validatePlanningSpec(spec PlanningSpec) error {
	if spec.Version != planningSpecVersion {
		return fmt.Errorf("unsupported planning spec version %d", spec.Version)
	}
	if len(spec.Steps) == 0 {
		return fmt.Errorf("planning spec requires at least one step")
	}
	ids := make(map[string]struct{}, len(spec.Steps))
	phases := make(map[phase.Phase]struct{}, len(spec.Steps))
	for i, step := range spec.Steps {
		label := fmt.Sprintf("step[%d]", i)
		if strings.TrimSpace(step.ID) == "" {
			return fmt.Errorf("%s id is required", label)
		}
		if err := validatePlanningStepID(step.ID); err != nil {
			return fmt.Errorf("%s id %q: %w", label, step.ID, err)
		}
		if _, ok := ids[step.ID]; ok {
			return fmt.Errorf("duplicate planning step id %q", step.ID)
		}
		ids[step.ID] = struct{}{}
		if strings.TrimSpace(step.Name) == "" {
			return fmt.Errorf("planning step %q name is required", step.ID)
		}
		if strings.TrimSpace(step.Prompt) == "" {
			return fmt.Errorf("planning step %q prompt is required", step.ID)
		}
		if _, err := normalizePlanningPromptPath(step.Prompt); err != nil {
			return fmt.Errorf("planning step %q prompt: %w", step.ID, err)
		}
		if strings.TrimSpace(step.Role) == "" {
			return fmt.Errorf("planning step %q role is required", step.ID)
		}
		if step.Gates == nil {
			return fmt.Errorf("planning step %q gates are required", step.ID)
		}
		if step.Gates.BeforeDispatch == nil || strings.TrimSpace(*step.Gates.BeforeDispatch) == "" {
			return fmt.Errorf("planning step %q gates.before_dispatch is required", step.ID)
		}
		parsedDispatch, err := phase.ParsePhase(*step.Gates.BeforeDispatch)
		if err != nil {
			return fmt.Errorf("planning step %q gates.before_dispatch: %w", step.ID, err)
		}
		if _, ok := phases[parsedDispatch]; ok {
			return fmt.Errorf("duplicate planning gate %q", *step.Gates.BeforeDispatch)
		}
		phases[parsedDispatch] = struct{}{}
		if step.Gates.BeforeAdvance == nil || strings.TrimSpace(*step.Gates.BeforeAdvance) == "" {
			return fmt.Errorf("planning step %q gates.before_advance is required", step.ID)
		}
		if _, err := phase.ParsePhase(*step.Gates.BeforeAdvance); err != nil {
			return fmt.Errorf("planning step %q gates.before_advance: %w", step.ID, err)
		}
	}
	return nil
}

// planningTaskFromSpec translates a planning spec into a planning task.
func planningTaskFromSpec(spec PlanningSpec) (planningTask, error) {
	ordered := make([]workstreamStep, 0, len(spec.Steps))
	byPhase := make(map[phase.Phase]workstreamStep, len(spec.Steps))
	for _, stepSpec := range spec.Steps {
		promptPath, _ := normalizePlanningPromptPath(stepSpec.Prompt)
		beforeDispatch, _ := phase.ParsePhase(*stepSpec.Gates.BeforeDispatch)
		beforeAdvance, _ := phase.ParsePhase(*stepSpec.Gates.BeforeAdvance)
		step := workstreamStep{
			phase:       beforeDispatch,
			name:        stepSpec.ID,
			displayName: stepSpec.Name,
			promptPath:  promptPath,
			role:        index.Role(stepSpec.Role),
			actions: workstreamStepActions{
				mergeToBase:  true,
				advancePhase: true,
			},
			gates: workstreamStepGates{
				beforeDispatch: workstreamGateTarget{enabled: true, phase: beforeDispatch},
				beforeAdvance:  workstreamGateTarget{enabled: true, phase: beforeAdvance},
			},
		}
		ordered = append(ordered, step)
		byPhase[beforeDispatch] = step
	}
	return planningTask{
		ordered: ordered,
		byPhase: byPhase,
	}, nil
}

// normalizePlanningPromptPath validates and normalizes a planning prompt path.
func normalizePlanningPromptPath(value string) (string, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "", fmt.Errorf("prompt path is required")
	}
	if strings.Contains(trimmed, "\\") {
		return "", fmt.Errorf("prompt path must use forward slashes")
	}
	if strings.HasPrefix(trimmed, "/") {
		return "", fmt.Errorf("prompt path must be relative")
	}
	cleaned := path.Clean(trimmed)
	if cleaned == "." || cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return "", fmt.Errorf("prompt path must not escape the repo")
	}
	return filepath.ToSlash(cleaned), nil
}

// validatePlanningStepID enforces the requirements for planning step ids.
func validatePlanningStepID(value string) error {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return fmt.Errorf("step id is required")
	}
	if strings.Contains(trimmed, "/") || strings.Contains(trimmed, "\\") {
		return fmt.Errorf("step id must not contain path separators")
	}
	if strings.Contains(trimmed, "..") {
		return fmt.Errorf("step id must not contain '..'")
	}
	return nil
}
