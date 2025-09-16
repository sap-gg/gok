package render

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/rs/zerolog/log"
	"github.com/sap-gg/gok/internal/strategy"
)

// Engine performs the rendering for manifest targets
type Engine struct {
	registry *strategy.Registry
	renderer *TemplateRenderer

	// manifestDir is the directory of the manifest.yaml
	manifestDir         string
	manifestDirResolver *GenericPathResolver

	// workDir is the directory where rendering output is placed
	workDir         string
	workDirResolver *GenericPathResolver
}

// NewEngine creates a new rendering engine. All parameters are required.
func NewEngine(manifestDir, workDir string, registry *strategy.Registry) (*Engine, error) {
	if manifestDir == "" || workDir == "" || registry == nil {
		return nil, fmt.Errorf("invalid engine config")
	}

	manifestDirResolver, err := NewGenericPathResolver(manifestDir)
	if err != nil {
		return nil, fmt.Errorf("manifest dir resolver: %w", err)
	}

	workDirResolver, err := NewGenericPathResolver(workDir)
	if err != nil {
		return nil, fmt.Errorf("work dir resolver: %w", err)
	}

	return &Engine{
		registry: registry,
		renderer: NewTemplateRenderer(),

		manifestDir:         manifestDir,
		manifestDirResolver: manifestDirResolver,

		workDir:         workDir,
		workDirResolver: workDirResolver,
	}, nil
}

func (e *Engine) RenderTargets(ctx context.Context, m *Manifest, targets []*ManifestTarget) error {
	allValues := PreprocessValues(m)

	var combined error
	for _, t := range targets {
		if err := e.renderOne(ctx, m, t, allValues); err != nil {
			log.Error().Err(err).Msgf("failed to render target %s", t.ID)
			combined = errors.Join(combined, fmt.Errorf("target %s: %w", t.ID, err))
		} else {
			log.Info().Msgf("successfully rendered target %s", t.ID)
		}
	}
	return combined
}

func (e *Engine) renderOne(ctx context.Context, m *Manifest, t *ManifestTarget, allValues ScopedValues) error {
	// construct the output directory. for example if the `-o test` flag is specified and the manifest.yaml
	// lies in /home/user/manifest.yaml and the target output is `out`, then
	// the final output directory is /home/user/test/out
	outputDir, err := e.workDirResolver.Resolve(t.Output)
	if err != nil {
		return fmt.Errorf("resolve output dir %q: %w", t.Output, err)
	}
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return fmt.Errorf("create output dir %q: %w", outputDir, err)
	}

	log.Info().Msgf("prepared output directory for %s: %q", t.ID, outputDir)

	currentOutputResolver, err := NewGenericPathResolver(outputDir)
	if err != nil {
		return fmt.Errorf("output dir resolver: %w", err)
	}

	tracker := NewTracker(currentOutputResolver)

	for _, templateSpec := range t.Templates {
		l := log.With().Str("template", templateSpec.Path).Logger()
		l.Info().Msg("processing template")

		// create context for template with all relevant values
		templateData := NewTemplateData(allValues, m.Globals, t, templateSpec)

		// srcRoot is the absolute path to the template source (file or directory)
		srcRoot, err := e.manifestDirResolver.Resolve(templateSpec.Path)
		if err != nil {
			return fmt.Errorf("resolve template input %q: %w", templateSpec, err)
		}

		info, err := os.Stat(srcRoot)
		if err != nil {
			l.Warn().
				Err(err).
				Msg("template path not accessible")
			return fmt.Errorf("stat template input %q: %w", srcRoot, err)
		}

		if !info.IsDir() {
			l.Warn().Msgf("template must be a directory, skipping")
			continue
		}

		if err := e.applyDir(ctx, srcRoot, currentOutputResolver, tracker, templateData); err != nil {
			return fmt.Errorf("apply dir %q: %w", srcRoot, err)
		}
	}

	// write lock file
	if err := tracker.WriteLock(); err != nil {
		log.Error().Err(err).Msg("failed to write lock file")
		return err
	}

	return nil
}

func (e *Engine) applyDir(
	ctx context.Context,
	srcDir string,
	dstDirResolver *GenericPathResolver,
	tracker *Tracker,
	data *TemplateData,
) error {
	return filepath.WalkDir(srcDir, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr // propagate the error
		}
		if d.IsDir() {
			return nil // skip directories as we only care about files and parents are created as needed
		}

		info, err := d.Info()
		if err != nil {
			return fmt.Errorf("get info for %q: %w", path, err)
		}
		if !info.Mode().IsRegular() {
			log.Debug().Str("path", path).Msg("skipping non-regular file")
			return nil // skip non-regular files
		}
		rel, err := filepath.Rel(srcDir, path)
		if err != nil {
			return fmt.Errorf("rel %q: %w", path, err)
		}

		dst, err := dstDirResolver.Resolve(rel)
		if err != nil {
			return fmt.Errorf("resolve dst %q: %w", rel, err)
		}

		return e.applyFile(ctx, path, dst, tracker, data)
	})
}

func (e *Engine) applyFile(ctx context.Context, src, dst string, tracker *Tracker, data *TemplateData) error {
	var (
		finalDst         = dst
		srcContentReader io.Reader
	)

	if strings.Contains(filepath.Base(src), ".templ") {
		log.Debug().Msgf("rendering template file %q...", src)
		finalDst = strings.Replace(dst, ".templ", "", 1)

		content, err := os.ReadFile(src)
		if err != nil {
			return fmt.Errorf("read template file %q: %w", src, err)
		}

		var renderedContent bytes.Buffer
		if err := e.renderer.Render(&renderedContent, string(content), data); err != nil {
			var execError template.ExecError
			if errors.As(err, &execError) {
				// TODO(future): pretty print
				log.Warn().Err(err).Msgf("template execution error")
			}
			return fmt.Errorf("render template %q: %w", src, err)
		}

		srcContentReader = &renderedContent
	} else {
		sf, err := os.Open(src)
		if err != nil {
			return fmt.Errorf("open src %q: %w", src, err)
		}
		defer sf.Close()
		srcContentReader = sf
	}

	var strat strategy.FileStrategy
	if _, err := os.Stat(finalDst); errors.Is(err, os.ErrNotExist) {
		// first seen: copy the (possibly rendered) content
		log.Debug().Msgf("destination %q does not exist, using fallback strategy", finalDst)
		strat = e.registry.Fallback()
	} else if err != nil {
		return fmt.Errorf("stat final dst %q: %w", finalDst, err)
	} else {
		var ok bool
		strat, ok = e.registry.For(finalDst)
		if !ok {
			strat = e.registry.Fallback()
			log.Debug().Msgf("no specific strategy for %q, using fallback %q", finalDst, strat.Name())
		} else {
			log.Debug().Msgf("using strategy %q for %q (by ext)", strat.Name(), finalDst)
		}
	}

	return strat.Apply(ctx, srcContentReader, finalDst, tracker)
}
