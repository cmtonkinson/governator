// Package run provides helpers for governator run orchestration safeguards.
package run

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/cmtonkinson/governator/internal/digests"
	"github.com/cmtonkinson/governator/internal/index"
)

// ErrPlanningDrift indicates stored planning digests no longer match the repo state.
var ErrPlanningDrift = errors.New("planning drift detected")

const adrDocsPrefix = "_governator/docs/adr/"

// ADRDriftReport summarizes ADR additions detected since the last planning digest refresh.
type ADRDriftReport struct {
	Added   []string
	Message string
}

// DetectADRDrift reports newly-added ADR files relative to stored digests.
func DetectADRDrift(repoRoot string, stored index.Digests) (ADRDriftReport, error) {
	current, err := digests.Compute(repoRoot)
	if err != nil {
		return ADRDriftReport{}, fmt.Errorf("detect ADR drift: %w", err)
	}

	added := adrAdditions(stored.PlanningDocs, current.PlanningDocs)
	message := ""
	if len(added) > 0 {
		message = formatADRDriftMessage(added)
	}

	return ADRDriftReport{
		Added:   added,
		Message: message,
	}, nil
}

// CheckPlanningDrift stops a run when new ADRs were added since planning.
func CheckPlanningDrift(repoRoot string, stored index.Digests) error {
	report, err := DetectADRDrift(repoRoot, stored)
	if err != nil {
		return fmt.Errorf("detect planning drift: %w", err)
	}
	if len(report.Added) == 0 {
		return nil
	}

	message := fmt.Sprintf("%s\nNext steps: rerun `governator start` to regenerate planning artifacts and the task index.", report.Message)
	return fmt.Errorf("%w: %s", ErrPlanningDrift, message)
}

func adrAdditions(storedDocs map[string]string, currentDocs map[string]string) []string {
	added := []string{}
	for path := range currentDocs {
		if !strings.HasPrefix(path, adrDocsPrefix) {
			continue
		}
		if _, ok := storedDocs[path]; !ok {
			added = append(added, path)
		}
	}
	sort.Strings(added)
	return added
}

func formatADRDriftMessage(added []string) string {
	builder := &strings.Builder{}
	builder.WriteString("ADR drift detected; replan required.")
	for _, path := range added {
		builder.WriteString("\n- ADR added: ")
		builder.WriteString(path)
	}
	return builder.String()
}
