// Package bootstrap provides helpers for generating Power Six artifacts.
package bootstrap

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/cmtonkinson/governator/internal/templates"
)

const (
	docsDirName      = "_governator/docs"
	templatesDirName = "_governator/templates"
	bootstrapRoot    = "bootstrap"
	docsDirMode      = 0o755
	artifactFileMode = 0o644
)

// Artifact describes a bootstrap artifact file and its template lookup key.
type Artifact struct {
	Name     string
	Template string
	Required bool
}

// Options configures bootstrap behavior.
type Options struct {
	Force bool
}

// Result captures which artifacts were written or skipped.
type Result struct {
	Written []string
	Skipped []string
}

// Artifacts returns the required and optional Power Six artifacts in stable order.
func Artifacts() []Artifact {
	return []Artifact{
		{Name: "asr.md", Template: "bootstrap/asr.md", Required: true},
		{Name: "arc42.md", Template: "bootstrap/arc42.md", Required: true},
		{Name: "adr.md", Template: "bootstrap/adr.md", Required: true},
		{Name: "personas.md", Template: "bootstrap/personas.md", Required: false},
		{Name: "wardley.md", Template: "bootstrap/wardley.md", Required: false},
		{Name: "c4.md", Template: "bootstrap/c4.md", Required: false},
	}
}

// Run ensures bootstrap artifacts exist under _governator/docs.
func Run(repoRoot string, options Options) (Result, error) {
	if strings.TrimSpace(repoRoot) == "" {
		return Result{}, errors.New("repo root is required")
	}
	return writeArtifacts(repoRoot, Artifacts(), options)
}

// writeArtifacts writes the provided artifacts to disk using templates.
func writeArtifacts(repoRoot string, artifacts []Artifact, options Options) (Result, error) {
	if len(artifacts) == 0 {
		return Result{}, errors.New("artifacts are required")
	}

	docsDir := filepath.Join(repoRoot, docsDirName)
	if err := os.MkdirAll(docsDir, docsDirMode); err != nil {
		return Result{}, fmt.Errorf("create docs directory %s: %w", docsDir, err)
	}

	result := Result{}
	for _, artifact := range artifacts {
		if strings.TrimSpace(artifact.Name) == "" {
			return Result{}, errors.New("artifact name is required")
		}
		if strings.TrimSpace(artifact.Template) == "" {
			return Result{}, errors.New("artifact template is required")
		}
		path := filepath.Join(docsDir, artifact.Name)
		if !options.Force {
			if _, err := os.Stat(path); err == nil {
				result.Skipped = append(result.Skipped, repoRelativePath(repoRoot, path))
				continue
			} else if !errors.Is(err, os.ErrNotExist) {
				return Result{}, fmt.Errorf("stat artifact %s: %w", path, err)
			}
		}

		data, err := loadTemplate(repoRoot, artifact.Template)
		if err != nil {
			return Result{}, fmt.Errorf("load template %s: %w", artifact.Template, err)
		}
		if err := os.WriteFile(path, data, artifactFileMode); err != nil {
			return Result{}, fmt.Errorf("write artifact %s: %w", path, err)
		}
		result.Written = append(result.Written, repoRelativePath(repoRoot, path))
	}

	return result, nil
}

// loadTemplate reads a template, preferring repo-local overrides when present.
func loadTemplate(repoRoot string, name string) ([]byte, error) {
	if strings.TrimSpace(repoRoot) == "" {
		return nil, errors.New("repo root is required")
	}
	if strings.TrimSpace(name) == "" {
		return nil, errors.New("template name is required")
	}

	localPath := filepath.Join(repoRoot, templatesDirName, filepath.FromSlash(name))
	info, err := os.Stat(localPath)
	if err == nil {
		if info.IsDir() {
			return nil, fmt.Errorf("template path is a directory: %s", localPath)
		}
		data, readErr := os.ReadFile(localPath)
		if readErr != nil {
			return nil, fmt.Errorf("read local template %s: %w", localPath, readErr)
		}
		return data, nil
	}
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("stat local template %s: %w", localPath, err)
	}

	return templates.Read(name)
}

// repoRelativePath returns a repository-relative path using forward slashes.
func repoRelativePath(root string, path string) string {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return filepath.ToSlash(path)
	}
	return filepath.ToSlash(rel)
}
