package render

import (
	"fmt"
	"path/filepath"
	"strings"
)

// PathResolver ensures paths are resolved correctly
type PathResolver struct {
	manifestDir string
	workDir     string
}

// NewPathResolver constructs a new resolver.
// manifestDir: absolute or relative base of manifest file
// workDir: absolute temp working tree
func NewPathResolver(manifestDir, workDir string) *PathResolver {
	return &PathResolver{manifestDir: manifestDir, workDir: workDir}
}

// ResolveTemplateInput resolves a template path (from manifest) to an absolute filesystem path.
// Rejects absolute inputs to avoid escaping the project.
func (r *PathResolver) ResolveTemplateInput(rel string) (string, error) {
	if filepath.IsAbs(rel) {
		return "", fmt.Errorf("template path must be relative: %q", rel)
	}
	p := filepath.Clean(filepath.Join(r.manifestDir, rel))
	return p, nil
}

// ResolveOutputDir resolves a target's output (from manifest) into an absolute path inside workDir.
// Rejects absolute outputs and path traversal outside workDir.
func (r *PathResolver) ResolveOutputDir(rel string) (string, error) {
	if filepath.IsAbs(rel) {
		return "", fmt.Errorf("output path must be relative: %q", rel)
	}
	out := filepath.Clean(filepath.Join(r.workDir, rel))

	// prevent escaping the workDir
	work := filepath.Clean(r.workDir)
	if !strings.HasPrefix(out+string(filepath.Separator), work+string(filepath.Separator)) && out != work {
		return "", fmt.Errorf("output path escapes workDir: %q", out)
	}
	return out, nil
}
