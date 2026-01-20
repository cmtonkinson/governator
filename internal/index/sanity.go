// Package index provides task index sanity checks.
package index

import (
	"fmt"
	"sort"
	"strings"
)

// SanityCheck inspects the index for obvious issues and reports warnings.
func SanityCheck(idx Index, warn func(string)) error {
	knownIDs := make(map[string]int, len(idx.Tasks))
	for _, task := range idx.Tasks {
		knownIDs[task.ID]++
		if !isKnownState(task.State) {
			emitWarning(warn, fmt.Sprintf("index sanity: task %q has unknown state %q", task.ID, task.State))
		}
	}

	duplicateIDs := duplicateIDList(knownIDs)
	for _, task := range idx.Tasks {
		for _, dependency := range task.Dependencies {
			if _, ok := knownIDs[dependency]; !ok {
				emitWarning(warn, fmt.Sprintf("index sanity: task %q references missing dependency %q", task.ID, dependency))
			}
		}
	}

	if len(duplicateIDs) > 0 {
		return fmt.Errorf("duplicate task ids: %s", strings.Join(duplicateIDs, ", "))
	}
	return nil
}

// isKnownState reports whether the provided state is recognized.
func isKnownState(state TaskState) bool {
	_, ok := knownTaskStates[state]
	return ok
}

// duplicateIDList returns a sorted list of ids with duplicate occurrences.
func duplicateIDList(idCounts map[string]int) []string {
	if len(idCounts) == 0 {
		return nil
	}

	var duplicates []string
	for id, count := range idCounts {
		if count > 1 {
			duplicates = append(duplicates, id)
		}
	}
	if len(duplicates) == 0 {
		return nil
	}
	sort.Strings(duplicates)
	return duplicates
}

// emitWarning forwards warnings to the provided sink.
func emitWarning(warn func(string), message string) {
	if warn == nil {
		return
	}
	warn(message)
}

// knownTaskStates enumerates the allowed task state values.
var knownTaskStates = map[TaskState]struct{}{
	TaskStateOpen:     {},
	TaskStateWorked:   {},
	TaskStateTested:   {},
	TaskStateDone:     {},
	TaskStateBlocked:  {},
	TaskStateConflict: {},
	TaskStateResolved: {},
}
