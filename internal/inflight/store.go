// Package inflight manages persisted tracking of active tasks.
package inflight

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const (
	localStateDirName = "_governator/_local-state"
	inFlightFileName  = "in-flight.json"
	inFlightFileMode  = 0o644
	localStateDirMode = 0o755
)

// Store provides access to persisted in-flight task tracking.
type Store struct {
	path string
}

// Entry captures per-task in-flight metadata.
type Entry struct {
	ID        string    `json:"id"`
	StartedAt time.Time `json:"started_at,omitempty"`
	Worktree  string    `json:"worktree_path,omitempty"`
}

// Set tracks in-flight task IDs in memory along with metadata.
type Set map[string]Entry

type snapshot struct {
	Tasks []Entry `json:"tasks"`
}

// NewStore builds a Store rooted at the provided repository root.
func NewStore(repoRoot string) (Store, error) {
	if repoRoot == "" {
		return Store{}, errors.New("repo root is required")
	}
	return Store{path: filepath.Join(repoRoot, localStateDirName, inFlightFileName)}, nil
}

// Load reads the in-flight task set from disk.
func (store Store) Load() (Set, error) {
	if store.path == "" {
		return nil, errors.New("in-flight store path is required")
	}

	data, err := os.ReadFile(store.path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Set{}, nil
		}
		return nil, fmt.Errorf("read in-flight data %s: %w", store.path, err)
	}
	if len(bytes.TrimSpace(data)) == 0 {
		return Set{}, nil
	}

	var snap snapshot
	if err := json.Unmarshal(data, &snap); err == nil {
		set := make(Set, len(snap.Tasks))
		for _, entry := range snap.Tasks {
			if entry.ID == "" {
				return nil, fmt.Errorf("decode in-flight data %s: empty task id", store.path)
			}
			set[entry.ID] = entry
		}
		return set, nil
	}

	var ids []string
	if err := json.Unmarshal(data, &ids); err != nil {
		return nil, fmt.Errorf("decode in-flight data %s: %w", store.path, err)
	}

	set := make(Set, len(ids))
	for _, id := range ids {
		if id == "" {
			return nil, fmt.Errorf("decode in-flight data %s: empty task id", store.path)
		}
		set[id] = Entry{ID: id}
	}
	return set, nil
}

// Save writes the in-flight task set to disk deterministically.
func (store Store) Save(set Set) error {
	if store.path == "" {
		return errors.New("in-flight store path is required")
	}
	if set == nil {
		return errors.New("in-flight set is required")
	}

	dir := filepath.Dir(store.path)
	if err := os.MkdirAll(dir, localStateDirMode); err != nil {
		return fmt.Errorf("create in-flight directory %s: %w", dir, err)
	}

	ids := set.IDs()
	tasks := make([]Entry, 0, len(ids))
	for _, id := range ids {
		entry := set[id]
		if entry.ID == "" {
			entry.ID = id
		}
		tasks = append(tasks, entry)
	}
	encoded, err := json.MarshalIndent(snapshot{Tasks: tasks}, "", "  ")
	if err != nil {
		return fmt.Errorf("encode in-flight data %s: %w", store.path, err)
	}
	if len(encoded) == 0 || encoded[len(encoded)-1] != '\n' {
		encoded = append(encoded, '\n')
	}
	if err := os.WriteFile(store.path, encoded, inFlightFileMode); err != nil {
		return fmt.Errorf("write in-flight data %s: %w", store.path, err)
	}
	return nil
}

// Add records task IDs as in-flight and persists the updated set.
func (store Store) Add(ids ...string) (Set, error) {
	set, err := store.Load()
	if err != nil {
		return nil, err
	}
	for _, id := range ids {
		if err := set.Add(id); err != nil {
			return nil, err
		}
	}
	if err := store.Save(set); err != nil {
		return nil, err
	}
	return set, nil
}

// Remove clears task IDs from in-flight tracking and persists the updated set.
func (store Store) Remove(ids ...string) (Set, error) {
	set, err := store.Load()
	if err != nil {
		return nil, err
	}
	for _, id := range ids {
		if err := set.Remove(id); err != nil {
			return nil, err
		}
	}
	if err := store.Save(set); err != nil {
		return nil, err
	}
	return set, nil
}

// Contains reports whether the set includes the provided task ID.
func (set Set) Contains(id string) bool {
	if set == nil {
		return false
	}
	_, ok := set[id]
	return ok
}

// StartedAt returns the recorded start time for a task when present.
func (set Set) StartedAt(id string) (time.Time, bool) {
	if set == nil {
		return time.Time{}, false
	}
	entry, ok := set[id]
	if !ok {
		return time.Time{}, false
	}
	if entry.StartedAt.IsZero() {
		return time.Time{}, false
	}
	return entry.StartedAt, true
}

// IDs returns the task IDs in sorted order.
func (set Set) IDs() []string {
	if len(set) == 0 {
		return []string{}
	}
	ids := make([]string, 0, len(set))
	for id := range set {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

// Add inserts a task ID into the set.
func (set Set) Add(id string) error {
	if id == "" {
		return errors.New("task id is required")
	}
	if set == nil {
		return errors.New("in-flight set is required")
	}
	entry, ok := set[id]
	if !ok {
		entry = Entry{ID: id, StartedAt: time.Now().UTC()}
		set[id] = entry
		return nil
	}
	if entry.ID == "" {
		entry.ID = id
	}
	if entry.StartedAt.IsZero() {
		entry.StartedAt = time.Now().UTC()
		set[id] = entry
	}
	return nil
}

// AddWithStart tracks a task ID with the provided start time.
func (set Set) AddWithStart(id string, startedAt time.Time) error {
	if id == "" {
		return errors.New("task id is required")
	}
	if set == nil {
		return errors.New("in-flight set is required")
	}
	if startedAt.IsZero() {
		startedAt = time.Now().UTC()
	}
	set[id] = Entry{ID: id, StartedAt: startedAt}
	return nil
}

// AddWithStartAndPath tracks a task ID with the provided start time and worktree path.
func (set Set) AddWithStartAndPath(id string, startedAt time.Time, worktreePath string) error {
	if id == "" {
		return errors.New("task id is required")
	}
	if set == nil {
		return errors.New("in-flight set is required")
	}
	if startedAt.IsZero() {
		startedAt = time.Now().UTC()
	}
	set[id] = Entry{ID: id, StartedAt: startedAt, Worktree: strings.TrimSpace(worktreePath)}
	return nil
}

// Remove deletes a task ID from the set when present.
func (set Set) Remove(id string) error {
	if id == "" {
		return errors.New("task id is required")
	}
	if set == nil {
		return errors.New("in-flight set is required")
	}
	delete(set, id)
	return nil
}

// WorktreePath returns the stored worktree path for a task when present.
func (set Set) WorktreePath(id string) (string, bool) {
	if set == nil {
		return "", false
	}
	entry, ok := set[id]
	if !ok {
		return "", false
	}
	if strings.TrimSpace(entry.Worktree) == "" {
		return "", false
	}
	return entry.Worktree, true
}
