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
)

const (
	planningSpecFilePath = "_governator/planning.json"
	planningSpecVersion  = 2
)

// PlanningSpec defines the JSON schema for the planning workstream.
type PlanningSpec struct {
	Version int                `json:"version"`
	Steps   []PlanningStepSpec `json:"steps"`
}

// PlanningStepSpec declares a single step in the planning workstream.
type PlanningStepSpec struct {
	ID         string                  `json:"id"`
	Name       string                  `json:"name"`
	Prompt     string                  `json:"prompt"`
	Role       string                  `json:"role"`
	Validations []PlanningValidationSpec `json:"validations,omitempty"`
}

// PlanningValidationSpec defines a validation check to run after step completion.
type PlanningValidationSpec struct {
	Type          string `json:"type"` // "command", "file", or "prompt"
	Command       string `json:"command,omitempty"`
	Expect        string `json:"expect,omitempty"` // for command: expected exit behavior
	StdoutRegex   string `json:"stdout_regex,omitempty"`
	StdoutContains string `json:"stdout_contains,omitempty"`
	Path          string `json:"path,omitempty"` // for file validation
	FileRegex     string `json:"regex,omitempty"` // for file content validation
	PromptRole    string `json:"role,omitempty"` // for prompt validation
	Inline        string `json:"inline,omitempty"` // for prompt validation
	PromptPath    string `json:"prompt_path,omitempty"` // for prompt validation
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
		if err := validatePlanningValidations(step.ID, step.Validations); err != nil {
			return fmt.Errorf("planning step %q validations: %w", step.ID, err)
		}
	}
	return nil
}

// validatePlanningValidations checks that validation specs are well-formed.
func validatePlanningValidations(stepID string, validations []PlanningValidationSpec) error {
	for i, validation := range validations {
		label := fmt.Sprintf("validation[%d]", i)
		if strings.TrimSpace(validation.Type) == "" {
			return fmt.Errorf("%s type is required", label)
		}
		
		switch validation.Type {
		case "command":
			if strings.TrimSpace(validation.Command) == "" {
				return fmt.Errorf("%s command is required for type 'command'", label)
			}
		case "file":
			if strings.TrimSpace(validation.Path) == "" {
				return fmt.Errorf("%s path is required for type 'file'", label)
			}
		case "prompt":
			if strings.TrimSpace(validation.Inline) == "" && strings.TrimSpace(validation.PromptPath) == "" {
				return fmt.Errorf("%s either inline or prompt_path is required for type 'prompt'", label)
			}
			if strings.TrimSpace(validation.Inline) != "" && strings.TrimSpace(validation.PromptPath) != "" {
				return fmt.Errorf("%s inline and prompt_path are mutually exclusive", label)
			}
			if strings.TrimSpace(validation.Inline) != "" && strings.TrimSpace(validation.PromptPath) != "" {
				return fmt.Errorf("%s inline and prompt_path are mutually exclusive", label)
			}
		default:
			return fmt.Errorf("%s unknown validation type %q", label, validation.Type)
		}
	}
	return nil
}

// planningTaskFromSpec translates a planning spec into a planning task.
func planningTaskFromSpec(spec PlanningSpec) (planningTask, error) {
	ordered := make([]workstreamStep, 0, len(spec.Steps))
	byID := make(map[string]workstreamStep, len(spec.Steps))
	
	for i, stepSpec := range spec.Steps {
		promptPath, _ := normalizePlanningPromptPath(stepSpec.Prompt)
		
		// Determine next step ID for sequencing
		var nextStepID string
		if i < len(spec.Steps)-1 {
			nextStepID = spec.Steps[i+1].ID
		} else {
			nextStepID = "execution" // Final step transitions to execution
		}
		
		step := workstreamStep{
			name:        stepSpec.ID,
			displayName: stepSpec.Name,
			promptPath:  promptPath,
			role:        index.Role(stepSpec.Role),
			validations: stepSpec.Validations,
			actions: workstreamStepActions{
				mergeToBase:  true,
				advancePhase: true,
			},
			nextStepID: nextStepID,
		}
		ordered = append(ordered, step)
		byID[stepSpec.ID] = step
	}
	return planningTask{
		ordered: ordered,
		byID:    byID,
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
