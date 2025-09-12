package render

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/sap-gg/gok/internal"
)

// Manifest represents the structure of the manifest file used to define rendering targets and their associated templates.
type Manifest struct {
	Targets map[string]*ManifestTarget `yaml:"targets"`
}

// ManifestTarget represents a single rendering target, including its output path and the list of templates to be applied.
type ManifestTarget struct {
	ID        string   `yaml:"-"`
	Output    string   `yaml:"output"`
	Templates []string `yaml:"templates"`
}

// Validate checks if the ManifestTarget has all required fields properly set.
func (t *ManifestTarget) Validate() error {
	if t.Output == "" {
		return fmt.Errorf("output is required")
	}
	if len(t.Templates) == 0 {
		return fmt.Errorf("at least one template is required")
	}
	for i, tmpl := range t.Templates {
		if tmpl == "" {
			return fmt.Errorf("template[%d]: path cannot be empty", i+1)
		}
	}
	return nil
}

// ReadManifest reads and parses a manifest file from the specified path, returning a Manifest struct.
func ReadManifest(ctx context.Context, path string) (*Manifest, string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, "", fmt.Errorf("open manifest %q: %w", path, err)
	}
	defer f.Close()

	var m Manifest
	if err := internal.NewYAMLDecoder(f).DecodeContext(ctx, &m); err != nil {
		return nil, "", fmt.Errorf("decode manifest: %w", err)
	}

	// some manifest validation
	if len(m.Targets) == 0 {
		return nil, "", fmt.Errorf("manifest has no targets")
	}
	for k, t := range m.Targets {
		if t == nil {
			return nil, "", fmt.Errorf("target %q is null", k)
		}
		if validateErr := t.Validate(); validateErr != nil {
			return nil, "", validateErr
		}
		t.ID = k
	}

	manifestDir := filepath.Dir(path)
	return &m, manifestDir, nil
}

// SelectTargets selects and returns the manifest targets based on the provided flags.
func SelectTargets(m *Manifest, all bool, names []string) ([]*ManifestTarget, error) {
	if all && len(names) > 0 {
		return nil, fmt.Errorf("cannot specify both all targets and specific target names")
	}

	var out []*ManifestTarget
	if all {
		for _, t := range m.Targets {
			out = append(out, t)
		}
		return out, nil
	}

	for _, name := range names {
		t, ok := m.Targets[name]
		if !ok {
			return nil, fmt.Errorf("target %q not found in manifest", name)
		}
		out = append(out, t)
	}

	return out, nil
}
