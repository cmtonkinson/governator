package phase

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/cmtonkinson/governator/internal/bootstrap"
)

const (
	docsDirName   = "_governator/docs"
	tasksDirName  = "_governator/tasks"
	gapFileName   = "gap-analysis.md"
	milestones    = "milestones.md"
	epics         = "epics.md"
	gapReportNote = "gap report"
)

// ArtifactValidation holds the result from validating a runnable phase gate.
type ArtifactValidation struct {
	Name      string    `json:"name"`
	Valid     bool      `json:"valid"`
	Message   string    `json:"message,omitempty"`
	CheckedAt time.Time `json:"checked_at"`
}

type phaseValidator func(repoRoot string) ([]ArtifactValidation, error)

var phaseValidators = map[Phase][]phaseValidator{
	PhaseGapAnalysis:     {validateArchitectureArtifacts},
	PhaseProjectPlanning: {validateGapReport},
	PhaseTaskPlanning:    {validateRoadmapArtifacts},
	PhaseExecution:       {validateTaskBacklog},
}

// ValidatePrerequisites inspects the repo and returns the list of artifact checks that gate the upcoming phase.
func ValidatePrerequisites(repoRoot string, upcoming Phase) ([]ArtifactValidation, error) {
	validators, ok := phaseValidators[upcoming]
	if !ok {
		return nil, nil
	}
	var results []ArtifactValidation
	for _, validator := range validators {
		entry, err := validator(repoRoot)
		if err != nil {
			return nil, err
		}
		results = append(results, entry...)
	}
	return results, nil
}

func validateArchitectureArtifacts(repoRoot string) ([]ArtifactValidation, error) {
	docsDir := filepath.Join(repoRoot, docsDirName)
	var validations []ArtifactValidation
	for _, artifact := range bootstrap.Artifacts() {
		if !artifact.Required {
			continue
		}
		path := filepath.Join(docsDir, artifact.Name)
		exists, err := fileExists(path)
		if err != nil {
			return nil, err
		}
		msg := ""
		if !exists {
			msg = fmt.Sprintf("missing %s", artifact.Name)
		}
		validations = append(validations, ArtifactValidation{
			Name:      filepath.Join(docsDirName, artifact.Name),
			Valid:     exists,
			Message:   msg,
			CheckedAt: time.Now(),
		})
	}
	return validations, nil
}

func validateGapReport(repoRoot string) ([]ArtifactValidation, error) {
	candidates := []string{
		filepath.Join(repoRoot, docsDirName, gapFileName),
		filepath.Join(repoRoot, docsDirName, "gap-report.md"),
		filepath.Join(repoRoot, gapFileName),
		filepath.Join(repoRoot, "gap-report.md"),
	}
	var missing []string
	for _, candidate := range candidates {
		exists, err := fileExists(candidate)
		if err != nil {
			return nil, err
		}
		if exists {
			return []ArtifactValidation{{
				Name:      filepath.Join(docsDirName, gapFileName),
				Valid:     true,
				Message:   fmt.Sprintf("found at %s", relativePath(repoRoot, candidate)),
				CheckedAt: time.Now(),
			}}, nil
		}
		missing = append(missing, relativePath(repoRoot, candidate))
	}
	return []ArtifactValidation{{
		Name:      gapReportNote,
		Valid:     false,
		Message:   fmt.Sprintf("missing gap report (%s)", strings.Join(missing, ", ")),
		CheckedAt: time.Now(),
	}}, nil
}

func validateRoadmapArtifacts(repoRoot string) ([]ArtifactValidation, error) {
	docsDir := filepath.Join(repoRoot, docsDirName)
	files := []string{milestones, epics}
	var validations []ArtifactValidation
	for _, name := range files {
		path := filepath.Join(docsDir, name)
		exists, err := fileExists(path)
		if err != nil {
			return nil, err
		}
		msg := ""
		if !exists {
			msg = fmt.Sprintf("missing %s", name)
		}
		validations = append(validations, ArtifactValidation{
			Name:      filepath.Join(docsDirName, name),
			Valid:     exists,
			Message:   msg,
			CheckedAt: time.Now(),
		})
	}
	return validations, nil
}

func validateTaskBacklog(repoRoot string) ([]ArtifactValidation, error) {
	directory := filepath.Join(repoRoot, tasksDirName)
	entries, err := os.ReadDir(directory)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return []ArtifactValidation{{
				Name:      tasksDirName,
				Valid:     false,
				Message:   "directory missing",
				CheckedAt: time.Now(),
			}}, nil
		}
		return nil, fmt.Errorf("read tasks directory: %w", err)
	}
	hasFile := false
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if strings.TrimSpace(entry.Name()) == "" {
			continue
		}
		hasFile = true
		break
	}
	msg := ""
	if !hasFile {
		msg = "no task markdown found"
	}
	return []ArtifactValidation{{
		Name:      tasksDirName,
		Valid:     hasFile,
		Message:   msg,
		CheckedAt: time.Now(),
	}}, nil
}

func fileExists(path string) (bool, error) {
	info, err := os.Stat(path)
	if err == nil {
		return !info.IsDir(), nil
	}
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	return false, fmt.Errorf("stat %s: %w", path, err)
}

func relativePath(root, target string) string {
	rel, err := filepath.Rel(root, target)
	if err != nil {
		return filepath.ToSlash(target)
	}
	return filepath.ToSlash(rel)
}
