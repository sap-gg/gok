package render

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"text/template"

	"github.com/rs/zerolog/log"

	"github.com/sap-gg/gok/internal"
)

// TemplateRenderer is responsible for parsing and executing Go templates.
// It caches parsed templates for reuse.
type TemplateRenderer struct {
	cache sync.Map // map[string]*template.Template
}

// NewTemplateRenderer creates a new TemplateRenderer.
func NewTemplateRenderer() *TemplateRenderer {
	return &TemplateRenderer{}
}

// Render parses and executes a template with the given data.
func (r *TemplateRenderer) Render(w io.Writer, content string, data any) error {
	tmpl, err := r.getTemplate(content)
	if err != nil {
		return err
	}
	log.Debug().Msgf("rendering content with data: %#v", data)
	return tmpl.Execute(w, data)
}

func (r *TemplateRenderer) getTemplate(content string) (*template.Template, error) {
	if cached, ok := r.cache.Load(content); ok {
		return cached.(*template.Template), nil
	}

	tmpl, err := template.New("gok").
		Option("missingkey=error").
		Parse(content)
	if err != nil {
		return nil, fmt.Errorf("parsing template: %w", err)
	}

	r.cache.Store(content, tmpl)
	return tmpl, nil
}

// TemplateSpec represents a single template specification, including the path to the template file.
type TemplateSpec struct {
	// The Path to the template, **relative to the manifest file**.
	Path string `yaml:"from"`

	// Values are additional values with a scope limited to this template.
	Values Values `yaml:"values"`
}

func (t *TemplateSpec) Validate() error {
	if t.Path == "" {
		return fmt.Errorf("template path is required")
	}
	return nil
}

// TemplateManifest defines the metadata for a template.
type TemplateManifest struct {
	Version     int            `yaml:"version"`
	Name        string         `yaml:"name"`
	Description string         `yaml:"description"`
	Maintainers []*Maintainer  `yaml:"maintainers"`
	Inherits    []*InheritSpec `yaml:"inherits"`
}

func (t *TemplateManifest) NameOrDefault(path string) string {
	if t == nil || t.Name == "" {
		return filepath.Base(path)
	}
	return t.Name
}

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

// InheritSpec defines a parent template to inherit from.
type InheritSpec struct {
	// Path to the parent template, **relative to the template's directory**
	Path string `yaml:"from"`

	// Values to pass to the inherited template
	Values Values `yaml:"values"`

	// Single marks the template to not apply if it was already applied
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

	return &m, nil
}
