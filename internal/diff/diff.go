package diff

import (
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"sort"

	"github.com/sap-gg/gok/internal/lockfile"
)

// Type represents the kind of Change detected for a file.
type Type int

const (
	Unchanged Type = iota
	Created
	Modified
	Removed
	Conflict
)

// Change represents the state change for a single file.
type Change struct {
	Type    Type
	Path    string
	OldHash string
	NewHash string
}

// Report contains the results of a diff operation.
type Report struct {
	Changes      map[string]*Change
	hasChanges   bool
	hasConflicts bool
}

// HasChanges returns true if there are any changes (created, modified, removed files).
func (r *Report) HasChanges() bool {
	return r.hasChanges
}

// HasConflicts returns true if there are any conflicts detected.
func (r *Report) HasConflicts() bool {
	return r.hasConflicts
}

// SortedPaths returns a sorted list of the file paths in the report.
func (r *Report) SortedPaths() []string {
	paths := make([]string, 0, len(r.Changes))
	for path := range r.Changes {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	return paths
}

// Comparer performs the comparison between current and desired states.
type Comparer struct {
	currentDir string // actual directory on disk
	desiredDir string // temporary directory with newly rendered files
}

// NewComparer creates a new Comparer instance.
func NewComparer(currentDir, desiredDir string) *Comparer {
	return &Comparer{
		currentDir: currentDir,
		desiredDir: desiredDir,
	}
}

// Compare performs the diff operation and returns a Report.
func (c *Comparer) Compare() (*Report, error) {
	oldLock, err := lockfile.Read(c.currentDir)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		// it's okay if the lock file doesn't exist (first run)
		return nil, err
	}

	newLock, err := lockfile.Read(c.desiredDir)
	if err != nil {
		return nil, fmt.Errorf("reading desired state lock file: %w", err)
	}

	report := &Report{
		Changes: make(map[string]*Change),
	}

	allPaths := getUnionKeys(oldLock.Files, newLock.Files)
	for _, path := range allPaths {
		oldEntry := oldLock.Files[path]
		newEntry := newLock.Files[path]

		currentPathOnDisk := filepath.Join(c.currentDir, path)
		actualHash, err := lockfile.FileSHA256(currentPathOnDisk)
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			return nil, fmt.Errorf("computing hash for %q: %w", currentPathOnDisk, err)
		}

		if oldEntry != nil && newEntry != nil {
			if oldEntry.Hash != actualHash {
				report.add(Conflict, path, oldEntry.Hash, newEntry.Hash)
			} else if oldEntry.Hash != newEntry.Hash {
				report.add(Modified, path, oldEntry.Hash, newEntry.Hash)
			} else {
				report.add(Unchanged, path, oldEntry.Hash, newEntry.Hash)
			}
		} else if oldEntry == nil && newEntry != nil {
			report.add(Created, path, "", newEntry.Hash)
		} else if oldEntry != nil {
			if actualHash != "" && oldEntry.Hash != actualHash {
				report.add(Conflict, path, oldEntry.Hash, "")
			} else {
				report.add(Removed, path, oldEntry.Hash, "")
			}
		}
	}

	return report, nil
}

func (r *Report) add(t Type, path, oldHash, newHash string) {
	if t == Unchanged {
		return
	}
	r.Changes[path] = &Change{
		Type:    t,
		Path:    path,
		OldHash: oldHash,
		NewHash: newHash,
	}
	r.hasChanges = true
	if t == Conflict {
		r.hasConflicts = true
	}
}

// getUnionKeys returns a slice of all unique keys present in either of the two maps.
func getUnionKeys[K comparable, V1, V2 any, M1 ~map[K]V1, M2 ~map[K]V2](m1 M1, m2 M2) []K {
	keySet := make(map[K]struct{})
	for k := range m1 {
		keySet[k] = struct{}{}
	}
	for k := range m2 {
		keySet[k] = struct{}{}
	}
	keys := make([]K, 0, len(keySet))
	for k := range keySet {
		keys = append(keys, k)
	}
	return keys
}
