package run

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/cmtonkinson/governator/internal/digests"
	"github.com/cmtonkinson/governator/internal/index"
)

const (
	tasksDirName      = "_governator/tasks"
	taskIndexFileName = "_governator/task-index.json"
	taskIndexSchema   = 1
)

func writeTestTaskFile(t *testing.T, repoRoot, id, title, role string) string {
	t.Helper()
	dir := filepath.Join(repoRoot, tasksDirName)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir tasks dir: %v", err)
	}
	filename := fmt.Sprintf("%s-%s-%s.md", id, slugify(title), role)
	path := filepath.Join(dir, filename)
	content := fmt.Sprintf("# %s\n\nRole: %s\n", title, role)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write task file %s: %v", path, err)
	}
	return filepath.ToSlash(filepath.Join(tasksDirName, filename))
}

func slugify(text string) string {
	clean := strings.TrimSpace(text)
	if clean == "" {
		return ""
	}
	builder := strings.Builder{}
	builder.Grow(len(clean))
	prevHyphen := false
	for _, r := range strings.ToLower(clean) {
		switch {
		case r >= 'a' && r <= 'z':
			builder.WriteRune(r)
			prevHyphen = false
		case r >= '0' && r <= '9':
			builder.WriteRune(r)
			prevHyphen = false
		default:
			if !prevHyphen {
				builder.WriteRune('-')
				prevHyphen = true
			}
		}
	}
	result := strings.Trim(builder.String(), "-")
	return result
}

func writeTestTaskIndex(t *testing.T, repoRoot string, tasks []index.Task) {
	t.Helper()
	digestsMap, err := digests.Compute(repoRoot)
	if err != nil {
		t.Fatalf("compute digests: %v", err)
	}
	idx := index.Index{
		SchemaVersion: taskIndexSchema,
		Digests:       digestsMap,
		Tasks:         tasks,
	}
	indexPath := filepath.Join(repoRoot, taskIndexFileName)
	if err := index.Save(indexPath, idx); err != nil {
		t.Fatalf("write task index %s: %v", indexPath, err)
	}
}

func newTestTask(id, title, role, path string, order int) index.Task {
	return index.Task{
		ID:           id,
		Title:        title,
		Path:         path,
		State:        index.TaskStateOpen,
		Role:         index.Role(role),
		Dependencies: []string{},
		Retries: index.RetryPolicy{
			MaxAttempts: 3,
		},
		Attempts: index.AttemptCounters{},
		Order:    order,
		Overlap:  []string{},
	}
}

func taskFilesInDir(repoRoot string) []string {
	var files []string
	dir := filepath.Join(repoRoot, tasksDirName)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		files = append(files, filepath.Join(tasksDirName, entry.Name()))
	}
	sort.Strings(files)
	return files
}
