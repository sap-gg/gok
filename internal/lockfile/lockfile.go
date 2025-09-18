package lockfile

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/goccy/go-yaml"
	"github.com/rs/zerolog/log"

	"github.com/sap-gg/gok/internal"
)

type LockFiles map[string]*LockEntry

func (l LockFiles) MarshalYAML() (interface{}, error) {
	var result yaml.MapSlice
	for k, v := range l {
		result = append(result, yaml.MapItem{
			Key:   k,
			Value: v,
		})
	}
	slices.SortFunc(result, func(a, b yaml.MapItem) int {
		return strings.Compare(a.Key.(string), b.Key.(string))
	})
	return result, nil
}

// LockFile represents the structure of the lock file used to record the state of rendered files.
type LockFile struct {
	Version     int       `yaml:"version"`
	GeneratedAt time.Time `yaml:"generatedAt"`
	Files       LockFiles `yaml:"files"`
}

// LockEntry contains metadata about a single file.
type LockEntry struct {
	Hash  string    `yaml:"hash"`
	MTime time.Time `yaml:"mtime"`
	Size  int64     `yaml:"size"`
}

func Create(ctx context.Context, rootDir string) error {
	log.Info().
		Str("root", rootDir).
		Msg("creating lock file")

	lock := LockFile{
		Version:     internal.LockFileVersion,
		GeneratedAt: time.Now().UTC(),
		Files:       make(LockFiles),
	}

	err := filepath.WalkDir(rootDir, func(path string, dir fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		// skip directories and the lock file itself
		if dir.IsDir() || dir.Name() == internal.LockFileName {
			return nil
		}

		relPath, err := filepath.Rel(rootDir, path)
		if err != nil {
			return fmt.Errorf("determining relative path: %w", err)
		}

		info, err := dir.Info()
		if err != nil {
			return fmt.Errorf("getting file info f or %q: %w", path, err)
		}

		hash, err := fileSHA256(path)
		if err != nil {
			return fmt.Errorf("computing hash for %q: %w", path, err)
		}

		lock.Files[filepath.ToSlash(relPath)] = &LockEntry{
			Hash:  hash,
			MTime: info.ModTime().UTC(),
			Size:  info.Size(),
		}

		return nil
	})

	if err != nil {
		return fmt.Errorf("walking root directory: %w", err)
	}

	lockPath := filepath.Join(rootDir, internal.LockFileName)
	f, err := os.Create(lockPath)
	if err != nil {
		return fmt.Errorf("creating lock file: %w", err)
	}
	defer f.Close()

	if err := internal.NewYAMLEncoder(f).EncodeContext(ctx, &lock); err != nil {
		return fmt.Errorf("encoding lock file: %w", err)
	}

	log.Info().
		Str("path", lockPath).
		Int("files", len(lock.Files)).
		Msg("lock file created successfully")
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
