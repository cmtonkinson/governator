// Package digests computes content digests for governance and planning artifacts.
package digests

import (
	"fmt"
	"sort"
	"strings"

	"github.com/cmtonkinson/governator/internal/index"
)

// DriftReport summarizes whether stored digests match the current repository state.
type DriftReport struct {
	HasDrift bool
	Message  string
	Details  []string
}

// Detect compares stored digests against the current repository and reports drift.
func Detect(repoRoot string, stored index.Digests) (DriftReport, error) {
	current, err := Compute(repoRoot)
	if err != nil {
		return DriftReport{}, err
	}

	reasons := driftReasons(stored, current)
	if len(reasons) == 0 {
		return DriftReport{
			HasDrift: false,
			Message:  "No planning drift detected.",
		}, nil
	}

	return DriftReport{
		HasDrift: true,
		Message:  formatDriftMessage(reasons),
		Details:  reasons,
	}, nil
}

func driftReasons(stored index.Digests, current index.Digests) []string {
	reasons := []string{}
	if stored.GovernatorMD != current.GovernatorMD {
		reasons = append(reasons, "GOVERNATOR.md changed")
	}

	storedDocs := stored.PlanningDocs
	currentDocs := current.PlanningDocs
	for path, storedDigest := range storedDocs {
		currentDigest, ok := currentDocs[path]
		if !ok {
			reasons = append(reasons, fmt.Sprintf("planning doc missing: %s", path))
			continue
		}
		if storedDigest != currentDigest {
			reasons = append(reasons, fmt.Sprintf("planning doc changed: %s", path))
		}
	}
	for path := range currentDocs {
		if _, ok := storedDocs[path]; !ok {
			reasons = append(reasons, fmt.Sprintf("planning doc added: %s", path))
		}
	}

	sort.Strings(reasons)
	return reasons
}

func formatDriftMessage(reasons []string) string {
	builder := &strings.Builder{}
	builder.WriteString("Planning drift detected; replan required.")
	for _, reason := range reasons {
		builder.WriteString("\n- ")
		builder.WriteString(reason)
	}
	return builder.String()
}
