// Package planner assembles planner prompt payloads and request data.
package planner

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/cmtonkinson/governator/internal/bootstrap"
	"github.com/cmtonkinson/governator/internal/config"
	"github.com/cmtonkinson/governator/internal/templates"
)

const (
	governatorPath       = "GOVERNATOR.md"
	docsDirName          = "_governator/docs"
	promptsDirName       = "_governator/prompts"
	templatesDirName     = "_governator/templates"
	plannerRequestKind   = "planner_request"
	plannerSchemaVersion = 1
)

var (
	planningSubjobTemplates = []string{
		"planning/architecture-baseline.md",
		"planning/gap-analysis.md",
		"planning/roadmap.md",
		"planning/tasks.md",
	}
	powerSixTitles = map[string]string{
		"asr.md":      "Architecturally Significant Requirements",
		"arc42.md":    "Architecture Overview",
		"adr.md":      "Architectural Decision Records",
		"personas.md": "User Personas",
		"wardley.md":  "Wardley Map",
		"c4.md":       "C4 Diagrams",
	}
)

// Document captures the planner request document shape for governance and Power Six inputs.
type Document struct {
	Path    string `json:"path"`
	Title   string `json:"title,omitempty"`
	Content string `json:"content"`
}

// RepoState captures repository summary data used during planning.
type RepoState struct {
	IsGreenfield bool     `json:"is_greenfield"`
	Summary      string   `json:"summary,omitempty"`
	Inventory    []string `json:"inventory,omitempty"`
}

// PlannerRequest is the JSON payload provided to the planner prompt.
type PlannerRequest struct {
	SchemaVersion int           `json:"schema_version"`
	Kind          string        `json:"kind"`
	GovernatorMD  Document      `json:"governator_md"`
	PowerSix      []Document    `json:"power_six"`
	Config        config.Config `json:"config"`
	RepoState     RepoState     `json:"repo_state"`
}

// PromptSection describes an ordered prompt section and its content.
type PromptSection struct {
	Title   string
	Content string
}

// AssemblePrompt builds the full planner prompt payload for the repository.
func AssemblePrompt(repoRoot string, cfg config.Config, repoState RepoState, warn func(string)) (string, error) {
	request, err := BuildPlannerRequest(repoRoot, cfg, repoState, warn)
	if err != nil {
		return "", err
	}
	globalPrompts, err := loadGlobalPrompts(repoRoot, warn)
	if err != nil {
		return "", err
	}
	subjobPrompts, err := loadPlanningSubjobs(repoRoot, repoState)
	if err != nil {
		return "", err
	}
	sections := append(globalPrompts, subjobPrompts...)
	return buildPromptPayload(sections, request)
}

// BuildPlannerRequest collects planner inputs from disk and returns a request payload.
func BuildPlannerRequest(repoRoot string, cfg config.Config, repoState RepoState, warn func(string)) (PlannerRequest, error) {
	if strings.TrimSpace(repoRoot) == "" {
		return PlannerRequest{}, errors.New("repo root is required")
	}
	governatorDoc, err := loadGovernatorDoc(repoRoot)
	if err != nil {
		return PlannerRequest{}, err
	}
	powerSix, err := loadPowerSixDocs(repoRoot, warn)
	if err != nil {
		return PlannerRequest{}, err
	}
	request := PlannerRequest{
		SchemaVersion: plannerSchemaVersion,
		Kind:          plannerRequestKind,
		GovernatorMD:  governatorDoc,
		PowerSix:      powerSix,
		Config:        cfg,
		RepoState:     repoState,
	}
	if err := request.Validate(); err != nil {
		return PlannerRequest{}, err
	}
	return request, nil
}

// EncodePlannerRequest renders the planner request as deterministic JSON.
func EncodePlannerRequest(request PlannerRequest) ([]byte, error) {
	if err := request.Validate(); err != nil {
		return nil, err
	}
	encoded := requestForEncode{
		SchemaVersion: request.SchemaVersion,
		Kind:          request.Kind,
		GovernatorMD:  request.GovernatorMD,
		PowerSix:      request.PowerSix,
		Config:        encodeConfig(request.Config),
		RepoState:     request.RepoState,
	}
	data, err := json.MarshalIndent(encoded, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("encode planner request: %w", err)
	}
	return data, nil
}

// Validate ensures the planner request satisfies the contract requirements.
func (request PlannerRequest) Validate() error {
	if request.SchemaVersion <= 0 {
		return errors.New("schema_version is required")
	}
	if strings.TrimSpace(request.Kind) != plannerRequestKind {
		return fmt.Errorf("kind must be %q", plannerRequestKind)
	}
	if err := validateDocument(request.GovernatorMD, "governator_md"); err != nil {
		return err
	}
	if len(request.PowerSix) == 0 {
		return errors.New("power_six is required")
	}
	for i, doc := range request.PowerSix {
		if err := validateDocument(doc, fmt.Sprintf("power_six[%d]", i)); err != nil {
			return err
		}
	}
	return nil
}

// loadGovernatorDoc reads the required GOVERNATOR.md file.
func loadGovernatorDoc(repoRoot string) (Document, error) {
	path := filepath.Join(repoRoot, governatorPath)
	data, err := os.ReadFile(path)
	if err != nil {
		return Document{}, fmt.Errorf("read %s: %w", path, err)
	}
	content := strings.TrimSpace(string(data))
	if content == "" {
		return Document{}, fmt.Errorf("%s is empty", path)
	}
	return Document{
		Path:    governatorPath,
		Content: content,
	}, nil
}

// loadPowerSixDocs returns Power Six artifacts in deterministic order.
func loadPowerSixDocs(repoRoot string, warn func(string)) ([]Document, error) {
	artifacts := bootstrap.Artifacts()
	powerSix := make([]Document, 0, len(artifacts))
	for _, artifact := range artifacts {
		path := filepath.Join(repoRoot, docsDirName, artifact.Name)
		content, ok, err := readOptionalFile(path)
		if err != nil {
			return nil, err
		}
		if !ok {
			if artifact.Required {
				return nil, fmt.Errorf("missing required Power Six doc %s", path)
			}
			emitWarning(warn, fmt.Sprintf("missing optional Power Six doc %s", path))
			continue
		}
		if content == "" {
			if artifact.Required {
				return nil, fmt.Errorf("required Power Six doc %s is empty", path)
			}
			emitWarning(warn, fmt.Sprintf("optional Power Six doc %s is empty", path))
			continue
		}
		rel := repoRelativePath(repoRoot, path)
		powerSix = append(powerSix, Document{
			Path:    rel,
			Title:   powerSixTitles[artifact.Name],
			Content: content,
		})
	}
	if len(powerSix) == 0 {
		return nil, errors.New("power six documents are required")
	}
	return powerSix, nil
}

// loadGlobalPrompts reads any optional global prompt files.
func loadGlobalPrompts(repoRoot string, warn func(string)) ([]PromptSection, error) {
	dir := filepath.Join(repoRoot, promptsDirName)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("read prompts dir %s: %w", dir, err)
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasPrefix(name, "_global") || !strings.HasSuffix(name, ".md") {
			continue
		}
		names = append(names, name)
	}
	sort.Strings(names)
	sections := make([]PromptSection, 0, len(names))
	for _, name := range names {
		path := filepath.Join(dir, name)
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read prompt %s: %w", path, err)
		}
		content := strings.TrimSpace(string(data))
		if content == "" {
			emitWarning(warn, fmt.Sprintf("global prompt %s is empty", path))
			continue
		}
		sections = append(sections, PromptSection{
			Title:   fmt.Sprintf("Global prompt: %s", name),
			Content: content,
		})
	}
	return sections, nil
}

// loadPlanningSubjobs loads prompt templates for each planning sub-job.
func loadPlanningSubjobs(repoRoot string, repoState RepoState) ([]PromptSection, error) {
	sections := []PromptSection{}
	for _, name := range planningSubjobTemplates {
		if repoState.IsGreenfield && strings.HasSuffix(name, "gap-analysis.md") {
			continue
		}
		content, err := loadTemplate(repoRoot, name)
		if err != nil {
			return nil, err
		}
		title := strings.TrimSuffix(filepath.Base(name), filepath.Ext(name))
		sections = append(sections, PromptSection{
			Title:   fmt.Sprintf("Planning sub-job: %s", title),
			Content: content,
		})
	}
	return sections, nil
}

// buildPromptPayload renders the ordered prompt sections and JSON input.
func buildPromptPayload(sections []PromptSection, request PlannerRequest) (string, error) {
	if len(sections) == 0 {
		return "", errors.New("prompt sections are required")
	}
	data, err := EncodePlannerRequest(request)
	if err != nil {
		return "", err
	}
	var builder strings.Builder
	for _, section := range sections {
		content := strings.TrimSpace(section.Content)
		if content == "" {
			return "", fmt.Errorf("prompt section %q is empty", section.Title)
		}
		builder.WriteString("## ")
		builder.WriteString(section.Title)
		builder.WriteString("\n")
		builder.WriteString(content)
		builder.WriteString("\n\n")
	}
	builder.WriteString("Input JSON:\n")
	builder.Write(data)
	builder.WriteString("\n")
	return builder.String(), nil
}

// requestForEncode ensures deterministic JSON encoding for planner requests.
type requestForEncode struct {
	SchemaVersion int             `json:"schema_version"`
	Kind          string          `json:"kind"`
	GovernatorMD  Document        `json:"governator_md"`
	PowerSix      []Document      `json:"power_six"`
	Config        configForEncode `json:"config"`
	RepoState     RepoState       `json:"repo_state"`
}

// configForEncode wraps config.Config with deterministic map encoders.
type configForEncode struct {
	Workers     workersForEncode       `json:"workers"`
	Concurrency concurrencyForEncode   `json:"concurrency"`
	Timeouts    config.TimeoutsConfig  `json:"timeouts"`
	Retries     config.RetriesConfig   `json:"retries"`
	AutoRerun   config.AutoRerunConfig `json:"auto_rerun"`
}

// workersForEncode wraps worker command maps for deterministic encoding.
type workersForEncode struct {
	Commands workerCommandsForEncode `json:"commands"`
}

// workerCommandsForEncode wraps role-specific worker commands.
type workerCommandsForEncode struct {
	Default []string       `json:"default"`
	Roles   stringSliceMap `json:"roles"`
}

// concurrencyForEncode wraps concurrency role caps for deterministic encoding.
type concurrencyForEncode struct {
	Global      int          `json:"global"`
	DefaultRole int          `json:"default_role"`
	Roles       stringIntMap `json:"roles"`
}

// stringSliceMap renders a map[string][]string with sorted keys.
type stringSliceMap map[string][]string

// MarshalJSON renders the map with deterministic key order.
func (values stringSliceMap) MarshalJSON() ([]byte, error) {
	if len(values) == 0 {
		return []byte("{}"), nil
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		if strings.TrimSpace(key) == "" {
			return nil, errors.New("map contains empty key")
		}
		keys = append(keys, key)
	}
	sort.Strings(keys)

	buffer := &bytes.Buffer{}
	buffer.WriteByte('{')
	for i, key := range keys {
		if i > 0 {
			buffer.WriteByte(',')
		}
		encodedKey, err := json.Marshal(key)
		if err != nil {
			return nil, fmt.Errorf("encode map key %q: %w", key, err)
		}
		value := values[key]
		encodedValue, err := json.Marshal(value)
		if err != nil {
			return nil, fmt.Errorf("encode map value for %q: %w", key, err)
		}
		buffer.Write(encodedKey)
		buffer.WriteByte(':')
		buffer.Write(encodedValue)
	}
	buffer.WriteByte('}')
	return buffer.Bytes(), nil
}

// stringIntMap renders a map[string]int with sorted keys.
type stringIntMap map[string]int

// MarshalJSON renders the map with deterministic key order.
func (values stringIntMap) MarshalJSON() ([]byte, error) {
	if len(values) == 0 {
		return []byte("{}"), nil
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		if strings.TrimSpace(key) == "" {
			return nil, errors.New("map contains empty key")
		}
		keys = append(keys, key)
	}
	sort.Strings(keys)

	buffer := &bytes.Buffer{}
	buffer.WriteByte('{')
	for i, key := range keys {
		if i > 0 {
			buffer.WriteByte(',')
		}
		encodedKey, err := json.Marshal(key)
		if err != nil {
			return nil, fmt.Errorf("encode map key %q: %w", key, err)
		}
		value := values[key]
		encodedValue, err := json.Marshal(value)
		if err != nil {
			return nil, fmt.Errorf("encode map value for %q: %w", key, err)
		}
		buffer.Write(encodedKey)
		buffer.WriteByte(':')
		buffer.Write(encodedValue)
	}
	buffer.WriteByte('}')
	return buffer.Bytes(), nil
}

// encodeConfig converts config.Config into a deterministic encoding wrapper.
func encodeConfig(cfg config.Config) configForEncode {
	return configForEncode{
		Workers: workersForEncode{
			Commands: workerCommandsForEncode{
				Default: cfg.Workers.Commands.Default,
				Roles:   stringSliceMap(cfg.Workers.Commands.Roles),
			},
		},
		Concurrency: concurrencyForEncode{
			Global:      cfg.Concurrency.Global,
			DefaultRole: cfg.Concurrency.DefaultRole,
			Roles:       stringIntMap(cfg.Concurrency.Roles),
		},
		Timeouts:  cfg.Timeouts,
		Retries:   cfg.Retries,
		AutoRerun: cfg.AutoRerun,
	}
}

// loadTemplate reads a planning template from repo overrides or embedded defaults.
func loadTemplate(repoRoot string, name string) (string, error) {
	if err := validateTemplateName(name); err != nil {
		return "", err
	}
	localPath := filepath.Join(repoRoot, templatesDirName, filepath.FromSlash(name))
	info, err := os.Stat(localPath)
	if err == nil {
		if info.IsDir() {
			return "", fmt.Errorf("template path is a directory: %s", localPath)
		}
		data, readErr := os.ReadFile(localPath)
		if readErr != nil {
			return "", fmt.Errorf("read template %s: %w", localPath, readErr)
		}
		content := strings.TrimSpace(string(data))
		if content == "" {
			return "", fmt.Errorf("template %s is empty", localPath)
		}
		return content, nil
	}
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return "", fmt.Errorf("stat template %s: %w", localPath, err)
	}

	data, err := templates.Read(name)
	if err != nil {
		return "", err
	}
	content := strings.TrimSpace(string(data))
	if content == "" {
		return "", fmt.Errorf("template %s is empty", name)
	}
	return content, nil
}

// validateTemplateName ensures template names stay within the planning namespace.
func validateTemplateName(name string) error {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return errors.New("template name is required")
	}
	if strings.Contains(trimmed, "\\") {
		return errors.New("template name must use forward slashes")
	}
	if strings.HasPrefix(trimmed, "/") {
		return errors.New("template name must be relative")
	}
	segments := strings.Split(trimmed, "/")
	for _, segment := range segments {
		if segment == "" || segment == "." || segment == ".." {
			return errors.New("template name contains invalid segments")
		}
	}
	if !strings.HasPrefix(trimmed, "planning/") {
		return errors.New("template name must start with planning/")
	}
	return nil
}

// validateDocument ensures required planner document fields are present.
func validateDocument(doc Document, label string) error {
	if strings.TrimSpace(doc.Path) == "" {
		return fmt.Errorf("%s.path is required", label)
	}
	if strings.TrimSpace(doc.Content) == "" {
		return fmt.Errorf("%s.content is required", label)
	}
	return nil
}

// readOptionalFile reads a file and returns whether it existed.
func readOptionalFile(path string) (string, bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return "", false, nil
		}
		return "", false, fmt.Errorf("read %s: %w", path, err)
	}
	return strings.TrimSpace(string(data)), true, nil
}

// repoRelativePath returns a repository-relative path using forward slashes.
func repoRelativePath(root string, path string) string {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return filepath.ToSlash(path)
	}
	return filepath.ToSlash(rel)
}

// emitWarning forwards warnings to the provided sink.
func emitWarning(warn func(string), message string) {
	if warn == nil {
		return
	}
	warn(message)
}
