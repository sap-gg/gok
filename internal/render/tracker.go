package render

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/sap-gg/gok/internal"
)

const (
	LockFileVersion = 1
	LockFileName    = "gok-lock.yaml"
)

// LockFile represents the structure of the lock file used to record the state of rendered files.
type LockFile struct {
	Version     int                   `yaml:"version"`
	GeneratedAt time.Time             `yaml:"generatedAt"`
	FilesMap    map[string]*LockEntry `yaml:"files"`
}

type LockEntry struct {
	Hash  string    `yaml:"hash"`
	MTime time.Time `yaml:"mtime"`
	Size  int64     `yaml:"size"`
}

///

type Tracker struct {
	resolver PathResolver
	affected map[string]struct{}
}

// NewTracker creates a new Tracker for the specified working directory.
func NewTracker(resolver PathResolver) *Tracker {
	return &Tracker{
		affected: make(map[string]struct{}),
		resolver: resolver,
	}
}

// Record marks the specified path as created / modified.
func (tr *Tracker) Record(absPath string) {
	tr.affected[filepath.Clean(absPath)] = struct{}{}
}

// Remove marks the specified path or directory as removed.
// If a directory is specified, all files under that directory are considered removed.
func (tr *Tracker) Remove(absPathOrDir string) {
	absPathOrDir = filepath.Clean(absPathOrDir)
	for p := range tr.affected {
		if p == absPathOrDir || strings.HasPrefix(p, absPathOrDir+string(os.PathSeparator)) {
			log.Debug().Msgf("removing tracked path because it got deleted via %s: %q",
				internal.DeletionFileName, p)
			delete(tr.affected, p)
		}
	}
}

// AffectedAbsolutePaths returns a stable, sorted list of affected absolute paths
func (tr *Tracker) AffectedAbsolutePaths() []string {
	paths := make([]string, 0, len(tr.affected))
	for p := range tr.affected {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	return paths
}

///

func (tr *Tracker) WriteLock() error {
	abs := tr.AffectedAbsolutePaths()
	log.Debug().
		Int("count", len(abs)).
		Msg("writing lock file for affected files...")

	// try to create the lock file
	lockPath, err := tr.resolver.Resolve(LockFileName)
	if err != nil {
		return fmt.Errorf("resolve lock file path: %w", err)
	}
	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return fmt.Errorf("create lock file %q: %w", lockPath, err)
	}
	defer f.Close()

	lock := LockFile{
		Version:     LockFileVersion,
		GeneratedAt: time.Now().UTC(),
		FilesMap:    make(map[string]*LockEntry),
	}

	for _, absolutePath := range abs {
		info, err := os.Stat(absolutePath)
		if err != nil {
			return fmt.Errorf("stat %q: %w", absolutePath, err)
		}
		if !info.Mode().IsRegular() {
			continue
		}

		sum, err := fileSHA256(absolutePath)
		if err != nil {
			return fmt.Errorf("hash %q: %w", absolutePath, err)
		}

		// we need to store the path relative path
		// to make it easier to compare across different machines
		rel, err := tr.resolver.Relative(absolutePath)
		if err != nil {
			return fmt.Errorf("rel %q: %w", absolutePath, err)
		}

		lock.FilesMap[rel] = &LockEntry{
			Hash:  sum,
			MTime: info.ModTime().UTC(),
			Size:  info.Size(),
		}
	}

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
