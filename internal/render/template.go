package render

import (
	"fmt"
	"io"
	"sync"
	"text/template"

	"github.com/rs/zerolog/log"
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
