package render

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/sap-gg/gok/internal"
)

const (
	LockFileVersion = 1
	LockFileName    = "gok-lock.yaml"
)

type Tracker struct {
	workDir  string
	affected map[string]struct{}
}

// NewTracker creates a new Tracker for the specified working directory.
func NewTracker(workDir string) *Tracker {
	return &Tracker{
		workDir:  workDir,
		affected: make(map[string]struct{}),
	}
}

// Record marks the specified path as created / modified.
func (tr *Tracker) Record(path string) {
	tr.affected[filepath.Clean(path)] = struct{}{}
}

// Affected returns a stable, sorted list of affected absolute paths
func (tr *Tracker) Affected() []string {
	paths := make([]string, 0, len(tr.affected))
	for p := range tr.affected {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	return paths
}

///

type LockFile struct {
	Version     int         `yaml:"version"`
	GeneratedAt time.Time   `yaml:"generatedAt"`
	Files       []LockEntry `yaml:"files"`
}

type LockEntry struct {
	Path  string    `yaml:"path"`
	Hash  string    `yaml:"hash"`
	MTime time.Time `yaml:"mtime"`
	Size  int64     `yaml:"size"`
}

func (tr *Tracker) WriteLock() error {
	abs := tr.Affected()
	log.Debug().
		Int("count", len(abs)).
		Msg("writing lock file for affected files")

	entries := make([]LockEntry, 0, len(abs))
	for _, p := range abs {
		info, err := os.Stat(p)
		if err != nil {
			return fmt.Errorf("stat %q: %w", p, err)
		}
		if !info.Mode().IsRegular() {
			continue
		}

		sum, err := fileSHA256(p)
		if err != nil {
			return fmt.Errorf("hash %q: %w", p, err)
		}
		rel, err := filepath.Rel(tr.workDir, p)
		if err != nil {
			return fmt.Errorf("rel %q: %w", p, err)
		}

		entries = append(entries, LockEntry{
			Path:  filepath.ToSlash(rel),
			Hash:  sum,
			MTime: info.ModTime().UTC(),
			Size:  info.Size(),
		})
	}

	lock := LockFile{
		Version:     LockFileVersion,
		GeneratedAt: time.Now().UTC(),
		Files:       entries,
	}

	lockPath := filepath.Join(tr.workDir, LockFileName)
	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return fmt.Errorf("create lock file %q: %w", lockPath, err)
	}
	defer f.Close()

	enc := internal.NewYAMLEncoder(f)
	if err := enc.Encode(lock); err != nil {
		return fmt.Errorf("encode lock file %q: %w", lockPath, err)
	}
	if err := enc.Close(); err != nil {
		return fmt.Errorf("write lock file %q: %w", lockPath, err)
	}

	log.Info().
		Str("file", lockPath).
		Msg("wrote lock file")
	return nil
}

// fileSHA256 computes the SHA256 hash of the file at the specified path and returns it as a hex string.
func fileSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
