package render

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/rs/zerolog/log"
)

// Engine performs the rendering for manifest targets
type Engine struct {
	resolver *PathResolver
	registry *StrategyRegistry
	tracker  *Tracker
}

type EngineConfig struct {
	ManifestDir string
	WorkDir     string
	Registry    *StrategyRegistry
}

// NewEngine creates a new rendering engine. All parameters are required.
func NewEngine(manifestDir, workDir string, registry *StrategyRegistry) (*Engine, error) {
	if manifestDir == "" || workDir == "" || registry == nil {
		return nil, fmt.Errorf("invalid engine config")
	}
	return &Engine{
		resolver: NewPathResolver(manifestDir, workDir),
		registry: registry,
		tracker:  NewTracker(workDir),
	}, nil
}

func (e *Engine) Tracker() *Tracker {
	return e.tracker
}

func (e *Engine) RenderTargets(ctx context.Context, targets []*ManifestTarget) error {
	var combined error
	for _, t := range targets {
		if err := e.renderOne(ctx, t); err != nil {
			log.Error().Err(err).
				Str("target", t.ID).
				Msg("render failed")
			combined = errors.Join(combined, fmt.Errorf("target %s: %w", t.ID, err))
		}
		log.Info().
			Str("target", t.ID).
			Msg("rendered target")
	}
	if err := e.tracker.WriteLock(); err != nil {
		log.Error().Err(err).Msg("failed to write lock file")
		combined = errors.Join(combined, err)
	}
	return combined
}

func (e *Engine) renderOne(ctx context.Context, t *ManifestTarget) error {
	outputDir, err := e.resolver.ResolveOutputDir(t.Output)
	if err != nil {
		return fmt.Errorf("resolve output dir: %w", err)
	}
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return fmt.Errorf("create output dir %q: %w", outputDir, err)
	}
	log.Info().
		Str("target", t.ID).
		Str("output", outputDir).
		Msg("prepared output directory")

	for _, raw := range t.Templates {
		log.Debug().
			Str("template", raw).
			Msg("processing template")

		srcRoot, err := e.resolver.ResolveTemplateInput(raw)
		if err != nil {
			return fmt.Errorf("resolve template input %q: %w", raw, err)
		}

		info, err := os.Stat(srcRoot)
		if err != nil {
			log.Warn().
				Err(err).
				Str("template", srcRoot).
				Msg("template path not accessible")
			return fmt.Errorf("stat template input %q: %w", srcRoot, err)
		}

		if info.IsDir() {
			if err := e.applyDir(ctx, srcRoot, outputDir); err != nil {
				return fmt.Errorf("apply dir %q: %w", srcRoot, err)
			}
		} else {
			dst := filepath.Join(outputDir, filepath.Base(srcRoot))
			if err := e.applyFile(ctx, srcRoot, dst); err != nil {
				return fmt.Errorf("apply file %q: %w", srcRoot, err)
			}
		}
	}

	return nil
}

func (e *Engine) applyDir(ctx context.Context, srcDir, dstDir string) error {
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

		dst := filepath.Join(dstDir, rel)
		return e.applyFile(ctx, path, dst)
	})
}

func (e *Engine) applyFile(ctx context.Context, src, dst string) error {
	strategy := e.registry.For(src)
	return strategy.Apply(ctx, src, dst, e.tracker)
}
