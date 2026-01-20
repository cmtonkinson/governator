// Package planner writes task files from planner output.
package planner

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const (
	tasksDirName     = "_governator/tasks"
	taskTemplateName = "planning/task.md"
	tasksDirMode     = 0o755
	taskFileMode     = 0o644
)

// TaskFileOptions configures task file writing behavior.
type TaskFileOptions struct {
	Force bool
}

// TaskFileResult reports which task files were written or skipped.
type TaskFileResult struct {
	Written []string
	Skipped []string
}

// WriteTaskFiles writes task markdown files based on planner output.
func WriteTaskFiles(repoRoot string, tasks []PlannedTask, options TaskFileOptions) (TaskFileResult, error) {
	if strings.TrimSpace(repoRoot) == "" {
		return TaskFileResult{}, errors.New("repo root is required")
	}
	if len(tasks) == 0 {
		return TaskFileResult{}, errors.New("tasks are required")
	}

	template, err := loadTemplate(repoRoot, taskTemplateName)
	if err != nil {
		return TaskFileResult{}, err
	}

	tasksDir := filepath.Join(repoRoot, tasksDirName)
	if err := os.MkdirAll(tasksDir, tasksDirMode); err != nil {
		return TaskFileResult{}, fmt.Errorf("create tasks directory %s: %w", tasksDir, err)
	}

	result := TaskFileResult{}
	seenIDs := map[string]struct{}{}
	seenPaths := map[string]struct{}{}
	for _, task := range tasks {
		if err := task.Validate(); err != nil {
			return TaskFileResult{}, err
		}
		if _, ok := seenIDs[task.ID]; ok {
			return TaskFileResult{}, fmt.Errorf("duplicate task id %q", task.ID)
		}
		seenIDs[task.ID] = struct{}{}

		filename := taskFileName(task.ID, task.Title)
		path := filepath.Join(tasksDir, filename)
		if _, ok := seenPaths[path]; ok {
			return TaskFileResult{}, fmt.Errorf("task file collision for %s", path)
		}
		seenPaths[path] = struct{}{}

		exists, err := fileExists(path)
		if err != nil {
			return TaskFileResult{}, fmt.Errorf("stat task file %s: %w", path, err)
		}
		if exists && !options.Force {
			result.Skipped = append(result.Skipped, repoRelativePath(repoRoot, path))
			continue
		}

		content, err := renderTaskFile(template, task)
		if err != nil {
			return TaskFileResult{}, err
		}
		if err := os.WriteFile(path, []byte(content), taskFileMode); err != nil {
			return TaskFileResult{}, fmt.Errorf("write task file %s: %w", path, err)
		}
		result.Written = append(result.Written, repoRelativePath(repoRoot, path))
	}

	sort.Strings(result.Written)
	sort.Strings(result.Skipped)
	return result, nil
}

// taskFileName builds a deterministic task filename from the task id and title.
func taskFileName(id string, title string) string {
	slug := slugifyTitle(title)
	base := id
	if slug != "" {
		base = id + "-" + slug
	}
	return base + ".md"
}

// slugifyTitle converts a title into a filesystem-safe lowercase slug.
func slugifyTitle(title string) string {
	trimmed := strings.TrimSpace(title)
	if trimmed == "" {
		return ""
	}
	var builder strings.Builder
	builder.Grow(len(trimmed))
	lastHyphen := false
	for _, r := range trimmed {
		if r >= 'A' && r <= 'Z' {
			r = r - 'A' + 'a'
		}
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			builder.WriteRune(r)
			lastHyphen = false
			continue
		}
		if !lastHyphen {
			builder.WriteByte('-')
			lastHyphen = true
		}
	}
	return strings.Trim(builder.String(), "-")
}

// renderTaskFile fills the task template with planner task data.
func renderTaskFile(template string, task PlannedTask) (string, error) {
	lines := strings.Split(template, "\n")
	updated, err := applyFrontMatter(lines, task)
	if err != nil {
		return "", err
	}
	updated, err = replaceLineWithPrefix(updated, "# Task:", "# Task: "+task.Title)
	if err != nil {
		return "", err
	}
	updated, err = replaceSection(updated, "## Objective", []string{task.Summary})
	if err != nil {
		return "", err
	}
	updated, err = replaceSection(updated, "## Context", []string{contextLine(task)})
	if err != nil {
		return "", err
	}
	updated, err = replaceSection(updated, "## Requirements", []string{"- [ ] Work satisfies the objective and acceptance criteria."})
	if err != nil {
		return "", err
	}
	updated, err = replaceSection(updated, "## Non-Goals", []string{"- None specified by planner."})
	if err != nil {
		return "", err
	}
	updated, err = replaceSection(updated, "## Constraints", []string{"- None specified by planner."})
	if err != nil {
		return "", err
	}
	updated, err = replaceSection(updated, "## Acceptance Criteria", checklistLines(task.AcceptanceCriteria, "- [ ] "))
	if err != nil {
		return "", err
	}
	updated, err = insertTestsSection(updated, task.Tests)
	if err != nil {
		return "", err
	}
	return strings.Join(updated, "\n"), nil
}

// applyFrontMatter updates task metadata in the template front matter.
func applyFrontMatter(lines []string, task PlannedTask) ([]string, error) {
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		return nil, errors.New("task template missing front matter")
	}
	end := -1
	for i := 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "---" {
			end = i
			break
		}
	}
	if end == -1 {
		return nil, errors.New("task template missing front matter end")
	}
	updated := append([]string(nil), lines...)
	for i := 1; i < end; i++ {
		line := strings.TrimSpace(updated[i])
		if strings.HasPrefix(line, "task:") {
			updated[i] = "task: " + task.ID
			continue
		}
		if strings.HasPrefix(line, "depends_on:") {
			updated[i] = "depends_on: " + formatDepends(task.Dependencies)
			continue
		}
	}
	return updated, nil
}

// formatDepends renders dependencies as an inline YAML list.
func formatDepends(dependencies []string) string {
	if len(dependencies) == 0 {
		return "[]"
	}
	items := make([]string, 0, len(dependencies))
	for _, dep := range dependencies {
		trimmed := strings.TrimSpace(dep)
		if trimmed == "" {
			continue
		}
		items = append(items, fmt.Sprintf("\"%s\"", trimmed))
	}
	if len(items) == 0 {
		return "[]"
	}
	return "[ " + strings.Join(items, ", ") + " ]"
}

// contextLine builds the default context content.
func contextLine(task PlannedTask) string {
	if len(task.Dependencies) == 0 {
		return "No explicit dependencies identified by the planner."
	}
	return "Depends on: " + strings.Join(task.Dependencies, ", ") + "."
}

// replaceLineWithPrefix replaces the first line with the provided prefix.
func replaceLineWithPrefix(lines []string, prefix string, replacement string) ([]string, error) {
	for i, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), prefix) {
			updated := append([]string(nil), lines...)
			updated[i] = replacement
			return updated, nil
		}
	}
	return nil, fmt.Errorf("task template missing %s", prefix)
}

// replaceSection replaces the content of a named section.
func replaceSection(lines []string, header string, content []string) ([]string, error) {
	start := -1
	for i, line := range lines {
		if strings.TrimSpace(line) == header {
			start = i
			break
		}
	}
	if start == -1 {
		return nil, fmt.Errorf("task template missing section %s", header)
	}
	end := len(lines)
	for i := start + 1; i < len(lines); i++ {
		trimmed := strings.TrimSpace(lines[i])
		if strings.HasPrefix(trimmed, "## ") || strings.HasPrefix(trimmed, "===") {
			end = i
			break
		}
	}
	block := sectionBlock(content)
	updated := make([]string, 0, len(lines)-((end-start)-len(block)))
	updated = append(updated, lines[:start+1]...)
	updated = append(updated, block...)
	updated = append(updated, lines[end:]...)
	return updated, nil
}

// sectionBlock wraps content with blank lines for readability.
func sectionBlock(content []string) []string {
	block := []string{""}
	block = append(block, content...)
	block = append(block, "")
	return block
}

// checklistLines formats checklist items, defaulting when empty.
func checklistLines(items []string, prefix string) []string {
	if len(items) == 0 {
		return []string{prefix + "None specified by planner."}
	}
	lines := make([]string, 0, len(items))
	for _, item := range items {
		trimmed := strings.TrimSpace(item)
		if trimmed == "" {
			continue
		}
		lines = append(lines, prefix+trimmed)
	}
	if len(lines) == 0 {
		return []string{prefix + "None specified by planner."}
	}
	return lines
}

// insertTestsSection injects a Tests section before the Notes section.
func insertTestsSection(lines []string, tests []string) ([]string, error) {
	insertAt := -1
	for i, line := range lines {
		if strings.TrimSpace(line) == "## Notes" {
			insertAt = i
			break
		}
	}
	if insertAt == -1 {
		return nil, errors.New("task template missing Notes section")
	}
	section := []string{"## Tests"}
	section = append(section, sectionBlock(checklistLines(tests, "- "))...)
	updated := make([]string, 0, len(lines)+len(section))
	updated = append(updated, lines[:insertAt]...)
	updated = append(updated, section...)
	updated = append(updated, lines[insertAt:]...)
	return updated, nil
}

// fileExists reports whether the provided path exists on disk.
func fileExists(path string) (bool, error) {
	if strings.TrimSpace(path) == "" {
		return false, errors.New("path is required")
	}
	_, err := os.Stat(path)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	return false, err
}
