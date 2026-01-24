// Package templates provides embedded template assets for bootstrap and planning.
package templates

import (
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"strings"
)

const (
	bootstrapRoot      = "bootstrap"
	planningRoot       = "planning"
	reasoningRoot      = "reasoning"
	rolesRoot          = "roles"
	customPromptsRoot  = "custom-prompts"
	workerContractName = "worker-contract.md"
)

//go:embed bootstrap/*.md planning/*.md reasoning/*.md roles/*.md custom-prompts/*.md worker-contract.md
var embeddedFS embed.FS

var requiredTemplates = []string{
	"bootstrap/asr.md",
	"bootstrap/arc42.md",
	"bootstrap/adr.md",
	"bootstrap/personas.md",
	"bootstrap/wardley.md",
	"bootstrap/c4.md",
	"planning/task.md",
	"planning/architecture-baseline.md",
	"planning/gap-analysis.md",
	"planning/roadmap.md",
	"planning/tasks.md",
	"roles/architect.md",
	"roles/default.md",
	"roles/planner.md",
	workerContractName,
}

// Required returns the template lookup keys required by bootstrap and planning.
func Required() []string {
	return append([]string(nil), requiredTemplates...)
}

// Read returns the embedded template contents for the provided lookup key.
func Read(name string) ([]byte, error) {
	cleaned, err := sanitizeName(name)
	if err != nil {
		return nil, err
	}

	data, err := fs.ReadFile(embeddedFS, cleaned)
	if err != nil {
		return nil, fmt.Errorf("read template %s: %w", cleaned, err)
	}
	return data, nil
}

// sanitizeName validates and normalizes template lookup keys.
func sanitizeName(name string) (string, error) {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return "", errors.New("template name is required")
	}
	if strings.HasPrefix(trimmed, "/") {
		return "", errors.New("template name must be relative")
	}
	if strings.Contains(trimmed, "\\") {
		return "", errors.New("template name must use forward slashes")
	}
	segments := strings.Split(trimmed, "/")
	for _, segment := range segments {
		if segment == "" {
			return "", errors.New("template name must not contain empty segments")
		}
		if segment == "." || segment == ".." {
			return "", errors.New("template name must not include dot segments")
		}
	}

	cleaned := path.Clean(trimmed)
	validRoot := strings.HasPrefix(cleaned, bootstrapRoot+"/") ||
		strings.HasPrefix(cleaned, planningRoot+"/") ||
		strings.HasPrefix(cleaned, reasoningRoot+"/") ||
		strings.HasPrefix(cleaned, rolesRoot+"/") ||
		strings.HasPrefix(cleaned, customPromptsRoot+"/") ||
		cleaned == workerContractName
	if !validRoot {
		return "", errors.New("template name must start with bootstrap/, planning/, reasoning/, roles/, custom-prompts/, or be " + workerContractName)
	}
	return cleaned, nil
}

// LocalFilename returns the flattened file name used under _governator/templates.
func LocalFilename(name string) string {
	return strings.ReplaceAll(name, "/", "-")
}
