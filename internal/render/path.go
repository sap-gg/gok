package render

import (
	"fmt"
	"path/filepath"
	"strings"
)

type PathResolver interface {
	Resolve(rel string) (string, error)
	Relative(abs string) (string, error)
}

///

var _ PathResolver = (*GenericPathResolver)(nil)

type GenericPathResolver struct {
	absoluteBaseDir string
}

// NewGenericPathResolver constructs a new resolver.
func NewGenericPathResolver(baseDir string) (*GenericPathResolver, error) {
	if baseDir == "" {
		return nil, fmt.Errorf("baseDir is required")
	}
	absBaseDir, err := filepath.Abs(baseDir)
	if err != nil {
		return nil, fmt.Errorf("failed to get absolute path of base dir %q: %w", baseDir, err)
	}
	return &GenericPathResolver{
		absoluteBaseDir: absBaseDir,
	}, nil
}

// Resolve resolves a path (relative to baseDir) to an absolute filesystem path.
func (r *GenericPathResolver) Resolve(rel string) (string, error) {
	return resolvePath(r.absoluteBaseDir, rel)
}

func (r *GenericPathResolver) Relative(abs string) (string, error) {
	abs = filepath.Clean(abs)
	cleanBaseDir := filepath.Clean(r.absoluteBaseDir)
	if !strings.HasPrefix(
		abs+string(filepath.Separator),
		cleanBaseDir+string(filepath.Separator),
	) && abs != cleanBaseDir {
		return "", fmt.Errorf("path %q is not within base dir: %q", abs, r.absoluteBaseDir)
	}
	rel, err := filepath.Rel(r.absoluteBaseDir, abs)
	if err != nil {
		return "", fmt.Errorf("failed to get relative path for %q from base dir %q: %w", abs, r.absoluteBaseDir, err)
	}
	return rel, nil
}

func resolvePath(baseDir, rel string) (string, error) {
	if filepath.IsAbs(rel) {
		return "", fmt.Errorf("path must be relative: %q", rel)
	}

	// resolved is the baseDir with the rel path joined and cleaned
	// e.g. baseDir="/home/user/project", rel="../etc/passwd"
	// results in "/home/user/etc/passwd"
	resolved := filepath.Clean(filepath.Join(baseDir, rel))

	// now we check if we escaped the base directory by checking if the resolved path starts with the baseDir
	// e.g. "/home/user/etc/passwd" does not start with "/home/user/project"
	// meaning we escaped the baseDir
	cleanBaseDir := filepath.Clean(baseDir)
	if !strings.HasPrefix(
		resolved+string(filepath.Separator),
		cleanBaseDir+string(filepath.Separator),
	) && resolved != cleanBaseDir {
		return "", fmt.Errorf("path %q escapes base dir: %q", resolved, baseDir)
	}

	return resolved, nil
}
