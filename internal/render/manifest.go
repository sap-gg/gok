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
	// Version indicates the version of the manifest format.
	// Currently, only version 1 is supported.
	Version int `yaml:"version"`

	// Values are values that are global to all templates in the manifest.
	// They may be overwritten by target-specific or template-specific values.
	Values Values `yaml:"values"`

	// Targets is a map of target names to their corresponding ManifestTarget definitions.
	Targets map[string]*ManifestTarget `yaml:"targets"`
}

// ManifestTarget represents a single rendering target, including its output path and the list of templates to be applied.
type ManifestTarget struct {
	// ID is an internal identifier, not part of the YAML manifest. It will be copied from the map key.
	ID string `yaml:"-"`

	// Tags are optional labels that can be used to categorize or filter targets.
	Tags []string

	// Output is the path where the rendered output will be saved.
	// Note that this does not mean file system where the output should be written to,
	// but rather a path inside the target "tarball" / output structure.
	Output string `yaml:"output" validate:"required"`

	// Templates is a list of templates to be applied for this target.
	// They will be applied from first to last, with later templates potentially overriding values from earlier ones.
	// Meaning that the last template has the highest precedence.
	Templates []*TemplateSpec `yaml:"templates"`

	// Values are additional values with a scope limited to this target.
	Values Values `yaml:"values"`
}

// GlobalSpec represents global values that can be applied to all templates in the manifest.
type GlobalSpec struct {
	// Values are global values available to all templates.
	Values Values
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
		if err := tmpl.Validate(); err != nil {
			return fmt.Errorf("template[%d]: %w", i+1, err)
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
		if internal.IsDecodeErrorAndPrint(err) {
			return nil, "", fmt.Errorf("parsing manifest")
		}
		return nil, "", fmt.Errorf("decode manifest: %w", err)
	}

	if m.Version != internal.ManifestVersion {
		return nil, "", fmt.Errorf("unsupported manifest version %d (expected %d)",
			m.Version, internal.ManifestVersion)
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
func SelectTargets(m *Manifest, all bool, names, tags []string) ([]*ManifestTarget, error) {
	if all {
		out := make([]*ManifestTarget, 0, len(m.Targets))
		for _, t := range m.Targets {
			out = append(out, t)
		}
		return out, nil
	}

	targetSet := make(map[string]*ManifestTarget)
	var targetOrder []*ManifestTarget

	// first add by name
	for _, name := range names {
		t, ok := m.Targets[name]
		if !ok {
			return nil, fmt.Errorf("target %q not found in manifest", name)
		}
		if _, exists := targetSet[t.ID]; !exists {
			targetOrder = append(targetOrder, t)
			targetSet[t.ID] = t
		}
	}

	// then add by tags
	for _, tag := range tags {
		for id, t := range m.Targets {
			for _, tTag := range t.Tags {
				if tTag == tag {
					if _, exists := targetSet[id]; !exists {
						targetOrder = append(targetOrder, t)
						targetSet[id] = t
					}
				}
			}
		}
	}

	return targetOrder, nil
}
