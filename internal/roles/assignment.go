// Package roles provides role prompt loading and stage-based role selection helpers.
package roles

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/cmtonkinson/governator/internal/index"
)

const (
	// roleAssignmentPromptPath is the repo-relative location of the role assignment prompt.
	roleAssignmentPromptPath = "_governator/prompts/role-assignment.md"
	// roleAssignmentAgentName is the audit agent name used for role selection.
	roleAssignmentAgentName = "role-assignment"
)

// LLMInvoker executes a prompt and returns the raw model response.
type LLMInvoker interface {
	Invoke(ctx context.Context, prompt string) (string, error)
}

// AuditLogger records audit events for role assignment.
type AuditLogger interface {
	LogAgentOutcome(taskID string, role string, agent string, status string, exitCode int) error
}

// RoleAssignmentTask captures the task metadata needed for role selection.
type RoleAssignmentTask struct {
	ID      string `json:"id"`
	Title   string `json:"title,omitempty"`
	Path    string `json:"path"`
	Content string `json:"content"`
}

// RoleAssignmentCaps captures concurrency caps and in-flight counts for role selection.
type RoleAssignmentCaps struct {
	Global      int        `json:"global"`
	DefaultRole int        `json:"default_role"`
	Roles       roleIntMap `json:"roles"`
	InFlight    roleIntMap `json:"in_flight"`
}

// RoleAssignmentRequest represents the JSON input sent to the role assignment LLM.
type RoleAssignmentRequest struct {
	Task           RoleAssignmentTask `json:"task"`
	Stage          Stage              `json:"stage"`
	AvailableRoles []index.Role       `json:"available_roles"`
	Caps           RoleAssignmentCaps `json:"caps"`
}

// RoleAssignmentResult captures the selected role and LLM rationale.
type RoleAssignmentResult struct {
	Role        index.Role
	Rationale   string
	RawResponse string
	Fallback    bool
}

// roleAssignmentResponse models the JSON response from the role assignment LLM.
type roleAssignmentResponse struct {
	Role      string `json:"role"`
	Rationale string `json:"rationale"`
}

// roleIntMap marshals role-to-int maps deterministically.
type roleIntMap map[index.Role]int

// MarshalJSON renders the map with sorted keys for deterministic output.
func (values roleIntMap) MarshalJSON() ([]byte, error) {
	if len(values) == 0 {
		return []byte("{}"), nil
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		if key == "" {
			return nil, errors.New("role map contains empty key")
		}
		keys = append(keys, string(key))
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
			return nil, fmt.Errorf("encode role key %q: %w", key, err)
		}
		value, ok := values[index.Role(key)]
		if !ok {
			continue
		}
		encodedValue, err := json.Marshal(value)
		if err != nil {
			return nil, fmt.Errorf("encode role cap for %q: %w", key, err)
		}
		buffer.Write(encodedKey)
		buffer.WriteByte(':')
		buffer.Write(encodedValue)
	}
	buffer.WriteByte('}')
	return buffer.Bytes(), nil
}

// Valid reports whether the stage is a supported role assignment stage.
func (stage Stage) Valid() bool {
	switch stage {
	case StageWork, StageTest, StageReview, StageResolve:
		return true
	default:
		return false
	}
}

// LoadRoleAssignmentPrompt reads the role assignment prompt from the repo root.
func LoadRoleAssignmentPrompt(root string) (string, error) {
	if strings.TrimSpace(root) == "" {
		return "", errors.New("root is required")
	}
	path := filepath.Join(root, roleAssignmentPromptPath)
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read role assignment prompt %s: %w", path, err)
	}
	if strings.TrimSpace(string(data)) == "" {
		return "", fmt.Errorf("role assignment prompt %s is empty", path)
	}
	return string(data), nil
}

// EncodeRoleAssignmentRequest serializes the role assignment request as JSON.
func EncodeRoleAssignmentRequest(request RoleAssignmentRequest) ([]byte, error) {
	if err := request.Validate(); err != nil {
		return nil, err
	}
	encoded, err := json.MarshalIndent(request, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("encode role assignment request: %w", err)
	}
	return encoded, nil
}

// BuildRoleAssignmentPrompt combines the prompt template with the request JSON.
func BuildRoleAssignmentPrompt(promptTemplate string, request RoleAssignmentRequest) (string, error) {
	promptTemplate = strings.TrimSpace(promptTemplate)
	if promptTemplate == "" {
		return "", errors.New("prompt template is required")
	}
	encoded, err := EncodeRoleAssignmentRequest(request)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s\n\nInput JSON:\n%s\n", promptTemplate, string(encoded)), nil
}

// SelectRole invokes the role assignment LLM to choose a role at dispatch time.
func SelectRole(
	ctx context.Context,
	invoker LLMInvoker,
	promptTemplate string,
	request RoleAssignmentRequest,
	warn func(string),
	auditor AuditLogger,
) (RoleAssignmentResult, error) {
	if invoker == nil {
		return RoleAssignmentResult{}, errors.New("llm invoker is required")
	}
	if err := request.Validate(); err != nil {
		return RoleAssignmentResult{}, err
	}
	prompt, err := BuildRoleAssignmentPrompt(promptTemplate, request)
	if err != nil {
		return RoleAssignmentResult{}, err
	}

	raw, err := invoker.Invoke(ctx, prompt)
	if err != nil {
		return fallbackRoleSelection(request, warn, auditor, "", err), nil
	}
	response, err := parseRoleAssignmentResponse(raw)
	if err != nil {
		return fallbackRoleSelection(request, warn, auditor, raw, err), nil
	}
	role := index.Role(strings.TrimSpace(response.Role))
	if !roleAllowed(request.AvailableRoles, role) {
		return fallbackRoleSelection(request, warn, auditor, raw, fmt.Errorf("role %q is not available", role)), nil
	}

	result := RoleAssignmentResult{
		Role:        role,
		Rationale:   strings.TrimSpace(response.Rationale),
		RawResponse: raw,
		Fallback:    false,
	}
	logRoleAssignmentOutcome(auditor, request.Task.ID, result.Role, "selected", 0)
	return result, nil
}

// Validate ensures the role assignment request meets the contract requirements.
func (request RoleAssignmentRequest) Validate() error {
	if strings.TrimSpace(request.Task.ID) == "" {
		return errors.New("task id is required")
	}
	if strings.TrimSpace(request.Task.Path) == "" {
		return errors.New("task path is required")
	}
	if strings.TrimSpace(request.Task.Content) == "" {
		return errors.New("task content is required")
	}
	if !request.Stage.Valid() {
		return fmt.Errorf("unsupported stage %q", request.Stage)
	}
	if len(request.AvailableRoles) == 0 {
		return errors.New("available roles are required")
	}
	for _, role := range request.AvailableRoles {
		if strings.TrimSpace(string(role)) == "" {
			return errors.New("available roles contain empty entry")
		}
	}
	if request.Caps.Global <= 0 {
		return errors.New("caps.global is required")
	}
	if request.Caps.DefaultRole <= 0 {
		return errors.New("caps.default_role is required")
	}
	for role, cap := range request.Caps.Roles {
		if strings.TrimSpace(string(role)) == "" {
			return errors.New("caps.roles contains empty role")
		}
		if cap < 0 {
			return fmt.Errorf("caps.roles.%s must be non-negative", role)
		}
	}
	for role, count := range request.Caps.InFlight {
		if strings.TrimSpace(string(role)) == "" {
			return errors.New("caps.in_flight contains empty role")
		}
		if count < 0 {
			return fmt.Errorf("caps.in_flight.%s must be non-negative", role)
		}
	}
	return nil
}

// parseRoleAssignmentResponse parses the LLM response JSON payload.
func parseRoleAssignmentResponse(raw string) (roleAssignmentResponse, error) {
	decoder := json.NewDecoder(strings.NewReader(raw))
	var response roleAssignmentResponse
	if err := decoder.Decode(&response); err != nil {
		return roleAssignmentResponse{}, fmt.Errorf("decode role assignment response: %w", err)
	}
	if err := ensureJSONEOF(decoder); err != nil {
		return roleAssignmentResponse{}, err
	}
	if strings.TrimSpace(response.Role) == "" {
		return roleAssignmentResponse{}, errors.New("role assignment response missing role")
	}
	if strings.TrimSpace(response.Rationale) == "" {
		return roleAssignmentResponse{}, errors.New("role assignment response missing rationale")
	}
	return response, nil
}

// ensureJSONEOF verifies the decoder consumed the entire JSON payload.
func ensureJSONEOF(decoder *json.Decoder) error {
	var extra any
	if err := decoder.Decode(&extra); err != nil {
		if errors.Is(err, io.EOF) {
			return nil
		}
		return err
	}
	return errors.New("invalid trailing content after JSON object")
}

// roleAllowed reports whether a role appears in the available roles list.
func roleAllowed(available []index.Role, role index.Role) bool {
	for _, candidate := range available {
		if candidate == role {
			return true
		}
	}
	return false
}

// fallbackRoleSelection applies the deterministic fallback role and logs a warning.
func fallbackRoleSelection(request RoleAssignmentRequest, warn func(string), auditor AuditLogger, raw string, cause error) RoleAssignmentResult {
	fallback := request.AvailableRoles[0]
	message := fmt.Sprintf("invalid role assignment response: %v; falling back to %s", cause, fallback)
	emitWarning(warn, message)
	logRoleAssignmentOutcome(auditor, request.Task.ID, fallback, "fallback", 1)
	return RoleAssignmentResult{
		Role:        fallback,
		RawResponse: raw,
		Fallback:    true,
	}
}

// logRoleAssignmentOutcome records the role assignment selection in the audit log.
func logRoleAssignmentOutcome(auditor AuditLogger, taskID string, role index.Role, status string, exitCode int) {
	if auditor == nil {
		return
	}
	_ = auditor.LogAgentOutcome(taskID, string(role), roleAssignmentAgentName, status, exitCode)
}
