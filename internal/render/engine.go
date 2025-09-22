package render

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"github.com/sap-gg/gok/internal"
	"github.com/sap-gg/gok/internal/artifact"
	"github.com/sap-gg/gok/internal/strategy"
	"github.com/sap-gg/gok/internal/templ"
)

// Engine performs the rendering for manifest targets
type Engine struct {
	registry        *strategy.Registry
	renderer        *templ.TemplateRenderer
	artifactTracker *artifact.Tracker

	globalValues         Values
	secretValues         Values
	externalFilesValues  *ValuesOverwritesSpec
	flagValueOverwrites  *ValuesOverwritesSpec
	resolvedTargetValues map[string]Values // for cross-target lookups only

	// manifestDir is the directory of the manifest.yaml
	manifestDir         string
	manifestDirResolver *GenericPathResolver

	// workDir is the directory where rendering output is placed
	workDir         string
	workDirResolver *GenericPathResolver
}

// NewEngine creates a new rendering engine. All parameters are required.
func NewEngine(
	manifestDir, workDir string,
	renderer *templ.TemplateRenderer,
	registry *strategy.Registry,
	globalValues Values,
	secretValues Values,
	externalFilesValues *ValuesOverwritesSpec,
	flagValueOverwrites *ValuesOverwritesSpec,
	resolvedTargetValues map[string]Values,
) (*Engine, error) {
	if manifestDir == "" {
		return nil, fmt.Errorf("manifest dir is required")
	}
	if workDir == "" {
		return nil, fmt.Errorf("work dir is required")
	}
	if registry == nil {
		return nil, fmt.Errorf("strategy registry is required")
	}

	artifactTracker, err := artifact.NewTracker()
	if err != nil {
		return nil, fmt.Errorf("artifact tracker: %w", err)
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
		registry:        registry,
		renderer:        renderer,
		artifactTracker: artifactTracker,

		globalValues:         globalValues,
		secretValues:         secretValues,
		externalFilesValues:  externalFilesValues,
		flagValueOverwrites:  flagValueOverwrites,
		resolvedTargetValues: resolvedTargetValues,

		manifestDir:         manifestDir,
		manifestDirResolver: manifestDirResolver,

		workDir:         workDir,
		workDirResolver: workDirResolver,
	}, nil
}

// RenderTargets renders the specified targets from the manifest.
// It continues rendering other targets even if one fails, and returns a combined error.
func (e *Engine) RenderTargets(ctx context.Context, targets []*ManifestTarget) error {
	// Pre-calculate the complete values map for cross-target value access
	var combined error
	for _, target := range targets {
		if err := e.RenderTarget(ctx, target); err != nil {
			log.Error().Err(err).Msgf("failed to render target %s", target.ID)
			combined = errors.Join(combined, fmt.Errorf("target %s: %w", target.ID, err))
		} else {
			log.Info().Msgf("successfully rendered target %s", target.ID)
		}
	}
	return combined
}

// RenderTarget renders a single target from the manifest.
func (e *Engine) RenderTarget(
	ctx context.Context,
	target *ManifestTarget,
) error {
	// create the output directory INSIDE the workDir
	outputDir, err := e.workDirResolver.Resolve(target.Output)
	if err != nil {
		return fmt.Errorf("resolve output dir %q: %w", target.Output, err)
	}
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return fmt.Errorf("create output dir %q: %w", outputDir, err)
	}
	log.Debug().Msgf("prepared output directory for %s: %q", target.ID, outputDir)

	currentOutputResolver, err := NewGenericPathResolver(outputDir)
	if err != nil {
		return fmt.Errorf("output dir resolver: %w", err)
	}

	for _, templateSpec := range target.Templates {
		if err := e.applyTemplate(ctx,
			target,
			templateSpec,
			currentOutputResolver,
		); err != nil {
			return fmt.Errorf("processing template spec %q: %w", templateSpec.Path, err)
		}
	}

	return nil
}

func (e *Engine) applyTemplate(
	ctx context.Context,
	target *ManifestTarget,
	templateSpec *TemplateSpec,
	currentOutputResolver *GenericPathResolver,
) error {
	l := log.With().Str("template", templateSpec.Path).Logger()

	// srcRoot is the absolute path to the template source (file or directory)
	srcRoot, err := e.manifestDirResolver.Resolve(templateSpec.Path)
	if err != nil {
		return fmt.Errorf("resolve template input %q: %w", templateSpec.Path, err)
	}

	templateManifest, err := ReadTemplateManifest(ctx, srcRoot)
	if err != nil {
		if internal.IsDecodeErrorAndPrint(err) {
			return fmt.Errorf("parsing manifest")
		}

		if !errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("read template manifest in %q: %w", srcRoot, err)
		}

		// it's okay if there's no manifest
		l.Debug().Msg("no template manifest found, proceeding without")
	} else {
		l.Debug().Msg("loaded template manifest")
	}

	l.Info().Msgf("processing template %s", templateManifest.NameOrDefault(srcRoot))
	if templateManifest != nil {
		if templateManifest.Description != "" {
			log.Info().Msgf(" ? %s", templateManifest.Description)
		}
		if len(templateManifest.Maintainers) > 0 {
			log.Info().Msgf(" ~ maintained by: %s", templateManifest.MaintainerString())
		}
	}

	availableValues := ComputeTemplateValues(e.globalValues,
		target.Values,
		templateSpec.Values,
		e.externalFilesValues.ValuesForTarget(target.ID),
		e.flagValueOverwrites.ValuesForTarget(target.ID),
	)

	templateContext, err := buildTemplateContext(
		l,
		templateManifest,
		target,
		availableValues,
		e.secretValues,
		e.resolvedTargetValues)
	if err != nil {
		return fmt.Errorf("build template context: %w", err)
	}

	info, err := os.Stat(srcRoot)
	if err != nil {
		return fmt.Errorf("stat template input %q: %w", srcRoot, err)
	}
	if !info.IsDir() {
		l.Warn().Msg("template path must be a directory, skipping")
		return nil
	}

	// apply deletions
	if err := e.applyDeletions(ctx, srcRoot, currentOutputResolver); err != nil {
		return fmt.Errorf("apply deletions for %q: %w", srcRoot, err)
	}

	if err := e.applyDir(ctx, srcRoot, currentOutputResolver, templateContext); err != nil {
		return fmt.Errorf("apply dir %q: %w", srcRoot, err)
	}

	return nil
}

func buildTemplateContext(
	l zerolog.Logger,
	templateManifest *TemplateManifest,
	target *ManifestTarget,
	availableValues Values,
	availableSecrets Values,
	allResolvedTargetValues map[string]Values,
) (Values, error) {
	if templateManifest == nil || templateManifest.Imports == nil {
		// no manifest / no imports, so no values ¯\_(ツ)_/¯
		return Values{}, nil
	}

	importedValues := make(Values)
	importedSecrets := make(Values)
	importedTargets := make(Values)
	var targetForTemplate *ManifestTarget

	// process non-sensitive value imports
	for key, req := range templateManifest.Imports.Values {
		l.Debug().Msgf("template requires value %q: %s", key, req.Description)

		val, found := LookupNestedValue(availableValues, key)
		var finalValue any
		if found {
			finalValue = val
		} else if req.Required {
			l.Error().Msgf("template requires value %q but it could not found", key)
			l.Error().Msgf(" ? %s", req.Description)
			return nil, fmt.Errorf("required external value %q not found", key)
		} else {
			l.Debug().Msgf("using default for non-required value %q", key)
			finalValue = req.Default
		}

		if err := SetNestedValue(importedValues, key, finalValue); err != nil {
			return nil, fmt.Errorf("set imported value %q: %w", key, err)
		}
	}

	// process sensitive value imports
	for key, req := range templateManifest.Imports.Secrets {
		l.Debug().Msgf("template requires secret %q: %s", key, req.Description)

		val, found := LookupNestedValue(availableSecrets, key)
		var finalValue any
		if found {
			finalValue = val
		} else if req.Required {
			l.Error().Msgf("template requires secret %q but it could not found", key)
			l.Error().Msgf(" ? %s", req.Description)
			return nil, fmt.Errorf("required secret %q not found", key)
		} else {
			l.Debug().Msgf("using default for non-required secret %q", key)
			finalValue = req.Default
		}

		if err := SetNestedValue(importedSecrets, key, finalValue); err != nil {
			return nil, fmt.Errorf("set imported secret %q: %w", key, err)
		}
	}

	// process target imports
	for targetID, req := range templateManifest.Imports.Targets {
		l.Debug().Msgf("template requires target %q: %s", targetID, req.Description)

		sourceTargetValues, ok := allResolvedTargetValues[targetID]
		if !ok {
			l.Error().Msgf("template requires target %q but it could not found", targetID)
			l.Error().Msgf(" ? %s", req.Description)
			return nil, fmt.Errorf("required target %q not found", targetID)
		}

		filteredTargetValues := make(Values)
		for key, valReq := range req.Values {
			val, found := LookupNestedValue(sourceTargetValues, key)
			var finalValue any
			if found {
				finalValue = val
			} else if valReq.Required {
				l.Error().Msgf("template requires value %q from target %q but it could not found", key, targetID)
				l.Error().Msgf(" ? %s", valReq.Description)
				return nil, fmt.Errorf("required value %q from target %q not found", key, targetID)
			} else {
				l.Debug().Msgf("using default for non-required value %q from target %q", key, targetID)
				finalValue = valReq.Default
			}
			if err := SetNestedValue(filteredTargetValues, key, finalValue); err != nil {
				return nil, fmt.Errorf("set imported target %q value %q: %w", targetID, key, err)
			}
		}

		if err := SetNestedValue(importedTargets, targetID, Values{
			"values": filteredTargetValues,
		}); err != nil {
			return nil, fmt.Errorf("set imported target %q: %w", targetID, err)
		}
	}

	// process target import
	if tt := templateManifest.Imports.Target; tt != nil {
		l.Debug().Msgf("template imports the whole target: %s", tt.Description)
		targetForTemplate = target
	}

	return Values{
		"values":  importedValues,
		"secrets": importedSecrets,
		"target":  targetForTemplate,
		"targets": importedTargets,
	}, nil
}

// DeletionSpec is the structure of the deletions file.
type DeletionSpec struct {
	Version   int         `yaml:"version"`
	Deletions []*Deletion `yaml:"deletions"`
}

// Deletion is a deletion entry in the deletions file.
type Deletion struct {
	Path      string `yaml:"path"`
	Recursive bool   `yaml:"recursive,omitempty"`
}

func (e *Engine) applyDeletions(
	ctx context.Context,
	srcRoot string,
	dstDirResolver *GenericPathResolver,
) error {
	deletionsFile := filepath.Join(srcRoot, internal.DeletionFileName)

	f, err := os.Open(deletionsFile)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			// that's fine, no deletions to apply
			log.Debug().Msgf("no deletions file %q, skipping deletions", deletionsFile)
			return nil
		}
		return fmt.Errorf("open deletions file %q: %w", deletionsFile, err)
	}

	var spec DeletionSpec
	if err := internal.NewYAMLDecoder(f).DecodeContext(ctx, &spec); err != nil {
		if internal.IsDecodeErrorAndPrint(err) {
			return fmt.Errorf("parsing deletions")
		}
		return fmt.Errorf("decode deletions file %q: %w", deletionsFile, err)
	}

	if spec.Version != internal.DeletionVersion {
		return fmt.Errorf("unsupported deletions version %d (expected %d)",
			spec.Version, internal.DeletionVersion)
	}

	log.Info().Msgf("applying %d deletions from %q...", len(spec.Deletions), internal.DeletionFileName)
	for _, deletion := range spec.Deletions {
		absPath, err := dstDirResolver.Resolve(deletion.Path)
		if err != nil {
			log.Warn().Err(err).Msgf("could not resolve deletion path %q", deletion.Path)
			continue
		}
		if deletion.Recursive {
			err = os.RemoveAll(absPath)
		} else {
			err = os.Remove(absPath)
		}
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				log.Warn().Msgf("file to delete does not exist, skipping: %q", absPath)
			} else {
				log.Warn().Err(err).Msgf("failed to delete path %q", absPath)
			}
		} else {
			log.Info().Msgf("deleted path %q", absPath)
		}
	}

	return nil
}

func (e *Engine) applyDir(
	ctx context.Context,
	srcDir string,
	dstDirResolver *GenericPathResolver,
	data any,
) error {
	return filepath.WalkDir(srcDir, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr // propagate the error
		}

		// skip any gok-related files
		baseName := filepath.Base(path)
		if baseName == internal.DeletionFileName || baseName == internal.TemplateManifestFileName {
			return nil // skip
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

		return e.applyFile(ctx, path, dst, data)
	})
}

func (e *Engine) applyFile(ctx context.Context, src, dst string, data any) error {
	var (
		finalDst         = dst
		srcContentReader io.Reader
	)

	base := filepath.Base(src)

	if strings.HasSuffix(base, internal.ArtifactSuffix) {
		finalDst = strings.TrimSuffix(dst, internal.ArtifactSuffix)
		log.Debug().Msgf("detected artifact manifest for %q", finalDst)

		content, err := os.ReadFile(src)
		if err != nil {
			return fmt.Errorf("read artifact manifest %q: %w", src, err)
		}

		var renderedContent bytes.Buffer

		// artifacts are always rendered using text/template
		if err := e.renderer.Render(&renderedContent, string(content), data); err != nil {
			return fmt.Errorf("render artifact manifest %q: %w", src, err)
		}

		// don't apply any file strategy, just register the artifact for later processing
		return e.artifactTracker.Register(finalDst, &renderedContent)
	}

	if strings.Contains(base, internal.TemplateInfix) {
		log.Debug().Msgf("rendering template file %q...", src)
		finalDst = strings.Replace(dst, internal.TemplateInfix, "", 1)

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
		log.Trace().Msgf("destination %q does not exist, using fallback strategy", finalDst)
		strat = e.registry.Fallback()
	} else if err != nil {
		return fmt.Errorf("stat final dst %q: %w", finalDst, err)
	} else {
		var ok bool
		strat, ok = e.registry.For(finalDst)
		if !ok {
			strat = e.registry.Fallback()
			log.Trace().Msgf("no specific strategy for %q, using fallback %q", finalDst, strat.Name())
		} else {
			log.Trace().Msgf("using strategy %q for %q (by ext)", strat.Name(), finalDst)
		}
	}

	return strat.Apply(ctx, srcContentReader, finalDst)
}

// ResolveArtifacts triggers the processing of all collected artifacts.
func (e *Engine) ResolveArtifacts(ctx context.Context) error {
	return e.artifactTracker.ProcessAll(ctx)
}
