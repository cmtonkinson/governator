// Package run contains tests for planning spec validation rules.
package run

import "testing"

func TestValidatePlanningValidationsRejectsUnexpectedFields(t *testing.T) {
	tests := []struct {
		name        string
		validations []PlanningValidationSpec
	}{
		{
			name: "directory rejects regex",
			validations: []PlanningValidationSpec{
				{Type: "directory", Path: "_governator/tasks", FileRegex: "x"},
			},
		},
		{
			name: "file rejects command",
			validations: []PlanningValidationSpec{
				{Type: "file", Path: "_governator/docs/doc.md", Command: "echo hi"},
			},
		},
		{
			name: "command rejects path",
			validations: []PlanningValidationSpec{
				{Type: "command", Command: "echo hi", Path: "_governator/docs/doc.md"},
			},
		},
		{
			name: "prompt rejects file regex",
			validations: []PlanningValidationSpec{
				{Type: "prompt", Inline: "check", FileRegex: "x"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := validatePlanningValidations("step", tt.validations); err == nil {
				t.Fatalf("expected validation error")
			}
		})
	}
}

func TestValidatePlanningValidationsAcceptsDirectoryPath(t *testing.T) {
	if err := validatePlanningValidations("step", []PlanningValidationSpec{
		{Type: "directory", Path: "_governator/tasks"},
	}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
