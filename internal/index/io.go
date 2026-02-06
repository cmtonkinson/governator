// Package index provides task index persistence helpers.
package index

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const indexFileMode os.FileMode = 0o644

// Load reads a task index JSON file from disk.
func Load(path string) (Index, error) {
	file, err := os.Open(path)
	if err != nil {
		return Index{}, fmt.Errorf("open task index %s: %w", path, err)
	}
	defer file.Close()

	idx, err := decodeIndex(file)
	if err != nil {
		return Index{}, fmt.Errorf("read task index %s: %w", path, err)
	}
	return idx, nil
}

// Save writes a task index JSON file to disk deterministically.
func Save(path string, idx Index) (err error) {
	// Ensure the parent directory exists
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create index directory %s: %w", dir, err)
	}

	lock, err := lockIndexForWrite(path)
	if err != nil {
		return err
	}
	defer func() {
		if lockErr := lock.Release(); lockErr != nil && err == nil {
			err = fmt.Errorf("release task index lock %s: %w", lock.path, lockErr)
		}
	}()

	encoded, err := encodeIndex(idx)
	if err != nil {
		return fmt.Errorf("encode task index %s: %w", path, err)
	}
	if len(encoded) == 0 || encoded[len(encoded)-1] != '\n' {
		encoded = append(encoded, '\n')
	}
	if err := os.WriteFile(path, encoded, indexFileMode); err != nil {
		return fmt.Errorf("write task index %s: %w", path, err)
	}
	return nil
}

// indexForWrite provides deterministic JSON output for the task index.
type indexForWrite struct {
	SchemaVersion int             `json:"schema_version"`
	Digests       digestsForWrite `json:"digests"`
	Tasks         []Task          `json:"tasks"`
}

// digestsForWrite encodes digests with deterministic planning document order.
type digestsForWrite struct {
	GovernatorMD string          `json:"governator_md"`
	PlanningDocs json.RawMessage `json:"planning_docs"`
}

// decodeIndex parses a task index from JSON.
func decodeIndex(reader io.Reader) (Index, error) {
	decoder := json.NewDecoder(reader)

	var idx Index
	if err := decoder.Decode(&idx); err != nil {
		return Index{}, fmt.Errorf("decode JSON: %w", err)
	}
	if err := ensureEOF(decoder); err != nil {
		return Index{}, err
	}
	for i := range idx.Tasks {
		idx.Tasks[i].State = normalizeTaskState(idx.Tasks[i].State)
	}
	return idx, nil
}

// ensureEOF verifies the decoder consumed the entire input.
func ensureEOF(decoder *json.Decoder) error {
	var extra any
	if err := decoder.Decode(&extra); err != nil {
		if errors.Is(err, io.EOF) {
			return nil
		}
		return err
	}
	return errors.New("invalid trailing content after JSON object")
}

// encodeIndex converts a task index into deterministic JSON bytes.
func encodeIndex(idx Index) ([]byte, error) {
	planningDocs, err := encodePlanningDocs(idx.Digests.PlanningDocs)
	if err != nil {
		return nil, err
	}

	wire := indexForWrite{
		SchemaVersion: idx.SchemaVersion,
		Digests: digestsForWrite{
			GovernatorMD: idx.Digests.GovernatorMD,
			PlanningDocs: planningDocs,
		},
		Tasks: normalizeTasksForWrite(idx.Tasks),
	}

	encoded, err := json.MarshalIndent(wire, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal index: %w", err)
	}
	return encoded, nil
}

// normalizeTasksForWrite returns a deterministic task list for JSON output.
func normalizeTasksForWrite(tasks []Task) []Task {
	if len(tasks) == 0 {
		return nil
	}

	normalized := make([]Task, len(tasks))
	for i, task := range tasks {
		normalized[i] = task
		normalized[i].Dependencies = sortedStrings(task.Dependencies)
		normalized[i].Overlap = sortedStrings(task.Overlap)
	}

	sort.SliceStable(normalized, func(i, j int) bool {
		left := normalized[i]
		right := normalized[j]
		if left.Order != right.Order {
			return left.Order < right.Order
		}
		if left.ID != right.ID {
			return left.ID < right.ID
		}
		if left.Path != right.Path {
			return left.Path < right.Path
		}
		return left.Role < right.Role
	})

	return normalized
}

// sortedStrings returns a sorted copy of the provided slice.
func sortedStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	normalized := make([]string, len(values))
	copy(normalized, values)
	sort.Strings(normalized)
	return normalized
}

func normalizeTaskState(raw TaskState) TaskState {
	switch strings.ToLower(string(raw)) {
	case "open":
		return TaskStateTriaged
	case "worked":
		return TaskStateImplemented
	case "done":
		return TaskStateMerged
	default:
		return raw
	}
}

// encodePlanningDocs sorts planning doc entries for stable JSON output.
func encodePlanningDocs(planningDocs map[string]string) (json.RawMessage, error) {
	if len(planningDocs) == 0 {
		return json.RawMessage([]byte("{}")), nil
	}

	keys := make([]string, 0, len(planningDocs))
	for key := range planningDocs {
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
			return nil, fmt.Errorf("encode planning doc key %q: %w", key, err)
		}
		encodedValue, err := json.Marshal(planningDocs[key])
		if err != nil {
			return nil, fmt.Errorf("encode planning doc value for %q: %w", key, err)
		}
		buffer.Write(encodedKey)
		buffer.WriteByte(':')
		buffer.Write(encodedValue)
	}
	buffer.WriteByte('}')
	return json.RawMessage(buffer.Bytes()), nil
}
