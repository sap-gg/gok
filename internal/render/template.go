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

const TemplateVersion = 1

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

	// Inherits is a list of parent templates to inherit from (optional)
	Inherits []*InheritSpec `yaml:"inherits"`

	// Imports is a list of values to receive from the manifest.
	// Only values specified here will be passed from the manifest to this template
	// and can be imported using {{ .values.some_key }}.
	Imports map[string]*TemplateValueRequirement `yaml:"imports"`
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

// TemplateValueRequirement defines a required value for a template.
type TemplateValueRequirement struct {
	// Description of the required value. This may be shown when printing help for a template or when it's missing.
	Description string

	// Mark this value as required. If a required value is missing, rendering should fail.
	Required bool

	// Default value if the value is not provided by the manifest and if Required is false.
	Default any
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

// InheritSpec defines a parent template to inherit from.
type InheritSpec struct {
	// Path to the parent template, **relative to the template's directory**
	Path string `yaml:"from"`

	// Values to pass to the inherited template
	Values Values `yaml:"values"`

	// Single marks the template to not apply if it was already applied before
	Single bool `yaml:"single"`
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
		return nil, fmt.Errorf("decode template manifest %q: %w", manifestPath, err)
	}

	if m.Version != TemplateVersion {
		return nil, fmt.Errorf("unsupported template manifest version %d (expected %d)", m.Version, TemplateVersion)
	}

	return &m, nil
}
