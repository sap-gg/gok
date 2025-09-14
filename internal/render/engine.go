package render

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/rs/zerolog/log"
	"github.com/sap-gg/gok/internal/strategy"
)

// Engine performs the rendering for manifest targets
type Engine struct {
	registry *strategy.Registry

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

		manifestDir:         manifestDir,
		manifestDirResolver: manifestDirResolver,

		workDir:         workDir,
		workDirResolver: workDirResolver,
	}, nil
}

func (e *Engine) RenderTargets(ctx context.Context, m *Manifest, targets []*ManifestTarget) error {
	var combined error
	for _, t := range targets {
		if err := e.renderOne(ctx, m, t); err != nil {
			log.Error().Err(err).Msgf("failed to render target %s", t.ID)
			combined = errors.Join(combined, fmt.Errorf("target %s: %w", t.ID, err))
		} else {
			log.Info().Msgf("successfully rendered target %s", t.ID)
		}
	}
	// TODO: re-enable lock file writing

	return combined
}

func (e *Engine) renderOne(ctx context.Context, m *Manifest, t *ManifestTarget) error {
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

	for _, raw := range t.Templates {
		log.Info().
			Str("template", raw).
			Msg("processing template")

		// srcRoot is the absolute path to the template source (file or directory)
		srcRoot, err := e.manifestDirResolver.Resolve(raw)
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

		if !info.IsDir() {
			log.Warn().
				Str("template", srcRoot).
				Msgf("template must be a directory, skipping")
			continue
		}

		if err := e.applyDir(ctx, m, srcRoot, currentOutputResolver, tracker); err != nil {
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

// isFileExcluded checks if the given path matches any of the exclude patterns.
func isFileExcluded(path string, excludes []string) bool {
	for _, pattern := range excludes {
		matched, err := filepath.Match(pattern, filepath.Base(path))
		if err == nil && matched {
			return true
		}
	}
	return false
}

func (e *Engine) applyDir(
	ctx context.Context,
	m *Manifest,
	srcDir string,
	dstDirResolver *GenericPathResolver,
	tracker *Tracker,
) error {
	return filepath.WalkDir(srcDir, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr // propagate the error
		}
		if d.IsDir() {
			return nil // skip directories as we only care about files and parents are created as needed
		}

		if isFileExcluded(path, m.Exclusions) {
			log.Info().Msgf("skipping excluded file: %q", path)
			return nil
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
		return e.applyFile(ctx, path, dst, tracker)
	})
}

func (e *Engine) applyFile(ctx context.Context, src, dst string, tracker *Tracker) error {
	var strat strategy.FileStrategy

	// if the destination file does not already exist, use the fallback strategy
	if _, err := os.Stat(dst); errors.Is(err, os.ErrNotExist) {
		log.Debug().Msgf("destination %q does not exist, using fallback strategy", dst)
		strat = e.registry.Fallback()
	} else {
		var ok bool
		strat, ok = e.registry.For(dst)
		if ok {
			log.Debug().Msgf("using strategy %q for %q (by ext)", strat.Name(), dst)
		}
	}

	return strat.Apply(ctx, src, dst, tracker)
}
