// Package planner parses planner output JSON into internal planning models.
package planner

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
)

const (
	plannerOutputKind        = "planner_output"
	architectureBaselineKind = "architecture_baseline"
	gapAnalysisKind          = "gap_analysis"
	roadmapKind              = "roadmap_decomposition"
	taskGenerationKind       = "task_generation"
)

// PlannerOutput captures the combined planner output payload.
type PlannerOutput struct {
	SchemaVersion        int                  `json:"schema_version"`
	Kind                 string               `json:"kind"`
	ArchitectureBaseline ArchitectureBaseline `json:"architecture_baseline"`
	GapAnalysis          *GapAnalysis         `json:"gap_analysis,omitempty"`
	Roadmap              Roadmap              `json:"roadmap"`
	Tasks                TaskGeneration       `json:"tasks"`
	Notes                []string             `json:"notes,omitempty"`
}

// ArchitectureBaseline captures the architecture baseline sub-job output.
type ArchitectureBaseline struct {
	SchemaVersion int      `json:"schema_version"`
	Kind          string   `json:"kind"`
	Mode          string   `json:"mode"`
	Summary       string   `json:"summary"`
	Components    []string `json:"components,omitempty"`
	Interfaces    []string `json:"interfaces,omitempty"`
	Constraints   []string `json:"constraints,omitempty"`
	Risks         []string `json:"risks,omitempty"`
	Assumptions   []string `json:"assumptions,omitempty"`
	Sources       []string `json:"sources"`
}

// GapAnalysis captures the gap analysis sub-job output.
type GapAnalysis struct {
	SchemaVersion int    `json:"schema_version"`
	Kind          string `json:"kind"`
	IsGreenfield  bool   `json:"is_greenfield"`
	Skipped       bool   `json:"skipped,omitempty"`
	Gaps          []Gap  `json:"gaps,omitempty"`
}

// Gap captures a single gap analysis entry.
type Gap struct {
	Area    string `json:"area"`
	Current string `json:"current"`
	Desired string `json:"desired"`
	Risk    string `json:"risk"`
}

// Roadmap captures the roadmap decomposition sub-job output.
type Roadmap struct {
	SchemaVersion int           `json:"schema_version"`
	Kind          string        `json:"kind"`
	DepthPolicy   string        `json:"depth_policy"`
	WidthPolicy   string        `json:"width_policy"`
	Items         []RoadmapItem `json:"items"`
}

// RoadmapItem describes a roadmap decomposition item.
type RoadmapItem struct {
	ID       string   `json:"id"`
	Title    string   `json:"title"`
	Type     string   `json:"type"`
	ParentID string   `json:"parent_id,omitempty"`
	Goal     string   `json:"goal,omitempty"`
	Order    int      `json:"order"`
	Overlap  []string `json:"overlap,omitempty"`
}

// TaskGeneration captures the task generation sub-job output.
type TaskGeneration struct {
	SchemaVersion int           `json:"schema_version"`
	Kind          string        `json:"kind"`
	Tasks         []PlannedTask `json:"tasks"`
}

// PlannedTask captures a generated task entry from the planner output.
type PlannedTask struct {
	ID                 string   `json:"id"`
	Title              string   `json:"title"`
	Summary            string   `json:"summary"`
	Role               string   `json:"role"`
	Dependencies       []string `json:"dependencies"`
	Order              int      `json:"order"`
	Overlap            []string `json:"overlap"`
	AcceptanceCriteria []string `json:"acceptance_criteria"`
	Tests              []string `json:"tests"`
}

// ParsePlannerOutput parses the combined planner output JSON payload.
func ParsePlannerOutput(data []byte) (PlannerOutput, error) {
	var output PlannerOutput
	if err := decodeJSON(data, "planner output", &output); err != nil {
		return PlannerOutput{}, err
	}
	if err := output.Validate(); err != nil {
		return PlannerOutput{}, err
	}
	output.normalize()
	return output, nil
}

// ParseArchitectureBaseline parses a standalone architecture baseline JSON payload.
func ParseArchitectureBaseline(data []byte) (ArchitectureBaseline, error) {
	var baseline ArchitectureBaseline
	if err := decodeJSON(data, "architecture baseline", &baseline); err != nil {
		return ArchitectureBaseline{}, err
	}
	if err := baseline.Validate(); err != nil {
		return ArchitectureBaseline{}, err
	}
	baseline.normalize()
	return baseline, nil
}

// ParseGapAnalysis parses a standalone gap analysis JSON payload.
func ParseGapAnalysis(data []byte) (GapAnalysis, error) {
	var analysis GapAnalysis
	if err := decodeJSON(data, "gap analysis", &analysis); err != nil {
		return GapAnalysis{}, err
	}
	if err := analysis.Validate(); err != nil {
		return GapAnalysis{}, err
	}
	analysis.normalize()
	return analysis, nil
}

// ParseRoadmap parses a standalone roadmap decomposition JSON payload.
func ParseRoadmap(data []byte) (Roadmap, error) {
	var roadmap Roadmap
	if err := decodeJSON(data, "roadmap", &roadmap); err != nil {
		return Roadmap{}, err
	}
	if err := roadmap.Validate(); err != nil {
		return Roadmap{}, err
	}
	roadmap.normalize()
	return roadmap, nil
}

// ParseTaskGeneration parses a standalone task generation JSON payload.
func ParseTaskGeneration(data []byte) (TaskGeneration, error) {
	var tasks TaskGeneration
	if err := decodeJSON(data, "task generation", &tasks); err != nil {
		return TaskGeneration{}, err
	}
	if err := tasks.Validate(); err != nil {
		return TaskGeneration{}, err
	}
	tasks.normalize()
	return tasks, nil
}

// Validate ensures the planner output satisfies the contract requirements.
func (output PlannerOutput) Validate() error {
	if output.SchemaVersion <= 0 {
		return errors.New("schema_version is required")
	}
	if strings.TrimSpace(output.Kind) != plannerOutputKind {
		return fmt.Errorf("kind must be %q", plannerOutputKind)
	}
	if err := output.ArchitectureBaseline.Validate(); err != nil {
		return fmt.Errorf("architecture_baseline: %w", err)
	}
	if output.GapAnalysis != nil {
		if err := output.GapAnalysis.Validate(); err != nil {
			return fmt.Errorf("gap_analysis: %w", err)
		}
	}
	if err := output.Roadmap.Validate(); err != nil {
		return fmt.Errorf("roadmap: %w", err)
	}
	if err := output.Tasks.Validate(); err != nil {
		return fmt.Errorf("tasks: %w", err)
	}
	return nil
}

// Validate ensures the architecture baseline payload is well-formed.
func (baseline ArchitectureBaseline) Validate() error {
	if baseline.SchemaVersion <= 0 {
		return errors.New("schema_version is required")
	}
	if strings.TrimSpace(baseline.Kind) != architectureBaselineKind {
		return fmt.Errorf("kind must be %q", architectureBaselineKind)
	}
	if err := requireString(baseline.Mode, "mode"); err != nil {
		return err
	}
	if err := requireString(baseline.Summary, "summary"); err != nil {
		return err
	}
	if err := requireStringSlice(baseline.Sources, "sources"); err != nil {
		return err
	}
	for i, source := range baseline.Sources {
		if err := requireString(source, fmt.Sprintf("sources[%d]", i)); err != nil {
			return err
		}
	}
	return nil
}

// Validate ensures the gap analysis payload is well-formed.
func (analysis GapAnalysis) Validate() error {
	if analysis.SchemaVersion <= 0 {
		return errors.New("schema_version is required")
	}
	if strings.TrimSpace(analysis.Kind) != gapAnalysisKind {
		return fmt.Errorf("kind must be %q", gapAnalysisKind)
	}
	if analysis.Gaps != nil {
		for i, gap := range analysis.Gaps {
			if err := gap.Validate(); err != nil {
				return fmt.Errorf("gaps[%d]: %w", i, err)
			}
		}
	}
	return nil
}

// Validate ensures the gap entry payload is well-formed.
func (gap Gap) Validate() error {
	if err := requireString(gap.Area, "area"); err != nil {
		return err
	}
	if err := requireString(gap.Current, "current"); err != nil {
		return err
	}
	if err := requireString(gap.Desired, "desired"); err != nil {
		return err
	}
	if err := requireString(gap.Risk, "risk"); err != nil {
		return err
	}
	return nil
}

// Validate ensures the roadmap payload is well-formed.
func (roadmap Roadmap) Validate() error {
	if roadmap.SchemaVersion <= 0 {
		return errors.New("schema_version is required")
	}
	if strings.TrimSpace(roadmap.Kind) != roadmapKind {
		return fmt.Errorf("kind must be %q", roadmapKind)
	}
	if err := requireString(roadmap.DepthPolicy, "depth_policy"); err != nil {
		return err
	}
	if err := requireString(roadmap.WidthPolicy, "width_policy"); err != nil {
		return err
	}
	if err := requireSlice(roadmap.Items, "items"); err != nil {
		return err
	}
	for i, item := range roadmap.Items {
		if err := item.Validate(); err != nil {
			return fmt.Errorf("items[%d]: %w", i, err)
		}
	}
	return nil
}

// Validate ensures the roadmap item payload is well-formed.
func (item RoadmapItem) Validate() error {
	if err := requireString(item.ID, "id"); err != nil {
		return err
	}
	if err := requireString(item.Title, "title"); err != nil {
		return err
	}
	if err := requireString(item.Type, "type"); err != nil {
		return err
	}
	if item.Order <= 0 {
		return errors.New("order is required")
	}
	if item.Overlap != nil {
		for i, label := range item.Overlap {
			if err := requireString(label, fmt.Sprintf("overlap[%d]", i)); err != nil {
				return err
			}
		}
	}
	return nil
}

// Validate ensures the task generation payload is well-formed.
func (generation TaskGeneration) Validate() error {
	if generation.SchemaVersion <= 0 {
		return errors.New("schema_version is required")
	}
	if strings.TrimSpace(generation.Kind) != taskGenerationKind {
		return fmt.Errorf("kind must be %q", taskGenerationKind)
	}
	if err := requireSlice(generation.Tasks, "tasks"); err != nil {
		return err
	}
	for i, task := range generation.Tasks {
		if err := task.Validate(); err != nil {
			return fmt.Errorf("tasks[%d]: %w", i, err)
		}
	}
	return nil
}

// Validate ensures the planned task payload is well-formed.
func (task PlannedTask) Validate() error {
	if err := requireString(task.ID, "id"); err != nil {
		return err
	}
	if err := requireString(task.Title, "title"); err != nil {
		return err
	}
	if err := requireString(task.Summary, "summary"); err != nil {
		return err
	}
	if err := requireString(task.Role, "role"); err != nil {
		return err
	}
	if err := requireStringSlice(task.Dependencies, "dependencies"); err != nil {
		return err
	}
	for i, dep := range task.Dependencies {
		if err := requireString(dep, fmt.Sprintf("dependencies[%d]", i)); err != nil {
			return err
		}
	}
	if task.Order <= 0 {
		return errors.New("order is required")
	}
	if err := requireStringSlice(task.Overlap, "overlap"); err != nil {
		return err
	}
	for i, label := range task.Overlap {
		if err := requireString(label, fmt.Sprintf("overlap[%d]", i)); err != nil {
			return err
		}
	}
	if err := requireStringSlice(task.AcceptanceCriteria, "acceptance_criteria"); err != nil {
		return err
	}
	if err := requireStringSlice(task.Tests, "tests"); err != nil {
		return err
	}
	return nil
}

func (output *PlannerOutput) normalize() {
	output.ArchitectureBaseline.normalize()
	if output.GapAnalysis != nil {
		output.GapAnalysis.normalize()
	}
	output.Roadmap.normalize()
	output.Tasks.normalize()
	if output.Notes == nil {
		output.Notes = []string{}
	}
}

func (baseline *ArchitectureBaseline) normalize() {
	if baseline.Components == nil {
		baseline.Components = []string{}
	}
	if baseline.Interfaces == nil {
		baseline.Interfaces = []string{}
	}
	if baseline.Constraints == nil {
		baseline.Constraints = []string{}
	}
	if baseline.Risks == nil {
		baseline.Risks = []string{}
	}
	if baseline.Assumptions == nil {
		baseline.Assumptions = []string{}
	}
}

func (analysis *GapAnalysis) normalize() {
	if analysis.Gaps == nil {
		analysis.Gaps = []Gap{}
	}
}

func (roadmap *Roadmap) normalize() {
	for i := range roadmap.Items {
		if roadmap.Items[i].Overlap == nil {
			roadmap.Items[i].Overlap = []string{}
		}
	}
}

func (generation *TaskGeneration) normalize() {
	for i := range generation.Tasks {
		task := &generation.Tasks[i]
		if task.Dependencies == nil {
			task.Dependencies = []string{}
		}
		if task.Overlap == nil {
			task.Overlap = []string{}
		}
		if task.AcceptanceCriteria == nil {
			task.AcceptanceCriteria = []string{}
		}
		if task.Tests == nil {
			task.Tests = []string{}
		}
	}
}

func decodeJSON(data []byte, label string, dest any) error {
	if len(bytes.TrimSpace(data)) == 0 {
		return fmt.Errorf("%s JSON is empty", label)
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	if err := decoder.Decode(dest); err != nil {
		return fmt.Errorf("decode %s JSON: %w", label, err)
	}
	if err := ensureEOF(decoder); err != nil {
		return fmt.Errorf("decode %s JSON: %w", label, err)
	}
	return nil
}

func ensureEOF(decoder *json.Decoder) error {
	var extra any
	if err := decoder.Decode(&extra); err != nil {
		if errors.Is(err, io.EOF) {
			return nil
		}
		return err
	}
	return errors.New("invalid trailing content after JSON object")
}

func requireString(value, field string) error {
	if strings.TrimSpace(value) == "" {
		return fmt.Errorf("%s is required", field)
	}
	return nil
}

func requireSlice[T any](value []T, field string) error {
	if value == nil {
		return fmt.Errorf("%s is required", field)
	}
	return nil
}

func requireStringSlice(value []string, field string) error {
	return requireSlice(value, field)
}
