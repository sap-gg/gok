package render

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/sap-gg/gok/internal"
)

// TemplateSpec represents a single template specification inside the manifest, including the path to the template file.
type TemplateSpec struct {
	// The Path to the template, **relative to the manifest file**
	Path string `yaml:"from"`

	// Values are additional values with a scope limited to this template
	Values Values `yaml:"values"`
}

func (t *TemplateSpec) Validate() error {
	if t.Path == "" {
		return fmt.Errorf("template path is required")
	}
	return nil
}

// TemplateManifest defines the structure of a template.yaml file inside a template directory.
type TemplateManifest struct {
	// Version of the template manifest format
	Version int `yaml:"version"`

	// Name of the template. (optional, default is the directory name)
	Name string `yaml:"name"`

	// Description of the template (optional)
	Description string `yaml:"description"`

	// Maintainers is a list of maintainers / responsible persons for this template (optional)
	Maintainers []*Maintainer `yaml:"maintainers"`

	// Imports is a list of values to receive from the manifest.
	// Only values specified here will be passed from the manifest to this template
	// (optional, default is to receive no values)
	Imports *TemplateImports `yaml:"imports"`
}

// NameOrDefault returns the template name, or the base name of the given path if the name is not set.
func (t *TemplateManifest) NameOrDefault(path string) string {
	if t == nil || t.Name == "" {
		return filepath.Base(path)
	}
	return t.Name
}

// MaintainerString returns a comma-separated string of maintainers.
func (t *TemplateManifest) MaintainerString() string {
	ms := make([]string, 0, len(t.Maintainers))
	for _, m := range t.Maintainers {
		ms = append(ms, m.String())
	}
	return strings.Join(ms, ", ")
}

// Maintainer represents a maintainer / responsible for a template.
type Maintainer struct {
	// Name of the maintainer
	Name string `json:"name" validate:"required"`
	// Email of the maintainer (optional)
	Email string `json:"email,omitempty"`
}

func (m *Maintainer) String() string {
	if m.Email != "" {
		return fmt.Sprintf("%s <%s>", m.Name, m.Email)
	}
	return m.Name
}

type TemplateImports struct {
	// Values to import from the manifest
	Values map[string]ValueImport `yaml:"values"`

	// Secrets to import from the manifest
	Secrets map[string]ValueImport `yaml:"secrets"`

	// Manifest indicates that the whole manifest should be imported
	Manifest *ReasonedImport `yaml:"manifest"`

	// Target indicates that the whole target should be imported
	Target *ReasonedImport `yaml:"target"`
}

// ValueImport defines a required (non-)sensitive value.
type ValueImport struct {
	// Description is the reasoning for importing this value (e.g. what it's used for)
	Description string `yaml:"description" validate:"required"`

	// Required marks this value as required. If a required value is missing, rendering should fail.
	Required bool `yaml:"required"`

	// Default is the default value if the value is not provided by the manifest and if Required is false.
	Default any `yaml:"default"`
}

// ReasonedImport defines an import which has a reasoning/description.
type ReasonedImport struct {
	// Description is the reasoning for importing the whole manifest (e.g. what it's used for)
	Description string `yaml:"description"`
}

// ReadTemplateManifest finds and parses a template.yaml file in a given directory.
// If the file does not exist, it returns (nil, fs.ErrNotExist)
func ReadTemplateManifest(ctx context.Context, dirPath string) (*TemplateManifest, error) {
	manifestPath := filepath.Join(dirPath, internal.TemplateManifestFileName)

	f, err := os.Open(manifestPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fs.ErrNotExist
		}
		return nil, fmt.Errorf("open template manifest %q: %w", manifestPath, err)
	}
	defer f.Close()

	var m TemplateManifest
	if err := internal.NewYAMLDecoder(f).DecodeContext(ctx, &m); err != nil {
		if internal.IsDecodeErrorAndPrint(err) {
			return nil, fmt.Errorf("parsing template manifest")
		}
		return nil, fmt.Errorf("decode template manifest %q: %w", manifestPath, err)
	}

	if m.Version != internal.TemplateManifestVersion {
		return nil, fmt.Errorf("unsupported template manifest version %d (expected %d)",
			m.Version, internal.TemplateManifestVersion)
	}

	return &m, nil
}
