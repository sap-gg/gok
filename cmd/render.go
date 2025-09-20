package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"

	"github.com/sap-gg/gok/internal"
	"github.com/sap-gg/gok/internal/archive"
	"github.com/sap-gg/gok/internal/lockfile"
	"github.com/sap-gg/gok/internal/logging"
	"github.com/sap-gg/gok/internal/render"
	"github.com/sap-gg/gok/internal/strategy"
	"github.com/sap-gg/gok/internal/templ"
)

var renderFlags = struct {
	manifestPath    string
	valuesFiles     []string // for external value files, merged from left to right
	secretFiles     []string
	valueOverwrites map[string]string

	// target selector flags:
	targets    []string
	tags       []string
	allTargets bool

	// output flags:
	outPath string // e.g. ./output.tar.gz or ./output-dir/
}{}

// renderCmd represents the render command
var renderCmd = &cobra.Command{
	Use:     "render -m <manifest> -t <target> ...",
	Short:   "Renders targets from a manifest by applying a series of templates and overlays.",
	Long:    renderLongDescription,
	Example: renderExample,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()

		// the output cannot already exist
		if renderFlags.outPath != "" {
			if _, err := os.Stat(renderFlags.outPath); err == nil {
				return fmt.Errorf("output path %q already exists", renderFlags.outPath)
			}
		}

		renderer := templ.NewTemplateRenderer()

		manifest, manifestDir, err := render.ReadManifest(ctx, renderFlags.manifestPath)
		if err != nil {
			return fmt.Errorf("reading manifest: %w", err)
		}

		// load any external values files (-f)
		externalValues, err := render.LoadValuesFiles(ctx, renderFlags.valuesFiles)
		if err != nil {
			return fmt.Errorf("loading external values files: %w", err)
		}

		valuesOverwries := make(render.Values)
		for k, v := range renderFlags.valueOverwrites {
			valuesOverwries[k] = v
		}

		secretValues, err := render.LoadValuesFiles(ctx, renderFlags.secretFiles)
		if err != nil {
			return fmt.Errorf("loading secret values files: %w", err)
		}
		sensitiveStrings := render.CollectStrings(secretValues)
		logging.Init(sensitiveStrings)
		log.Debug().Int("count", len(sensitiveStrings)).
			Msg("initialized logging with sensitive values redaction")

		// select which targets to render
		targets, err := render.SelectTargets(manifest, renderFlags.allTargets, renderFlags.targets, renderFlags.tags)
		if err != nil {
			return fmt.Errorf("selecting targets: %w", err)
		}
		if len(targets) == 0 {
			return fmt.Errorf("no targets matched the selection criteria")
		}
		for _, t := range targets {
			log.Info().Msgf("selected render target: %s", t.ID)
		}

		// rendering always happens in a temporary directory, and this directory will _always_ be deleted after rendering
		// when --no-compress and an output is specified, this directory will be _moved_ after rendering
		workDir, err := os.MkdirTemp("", "gok-workdir-")
		if err != nil {
			return fmt.Errorf("creating working directory: %w", err)
		}
		workDirMoved := false
		defer func() {
			if workDirMoved {
				log.Debug().Msg("work directory was moved, not deleting")
				return
			}
			if rmErr := os.RemoveAll(workDir); rmErr != nil {
				log.Debug().Err(rmErr).Msg("failed to remove temporary directory")
			} else {
				log.Info().Str("dir", workDir).Msg("removed temporary directory")
			}
		}()
		log.Debug().Msgf("created temporary directory: %s", workDir)

		registry, err := newStrategyRegistry()
		if err != nil {
			return fmt.Errorf("creating strategy registry: %w", err)
		}

		engine, err := render.NewEngine(manifestDir,
			workDir,
			renderer,
			registry,
			externalValues,
			secretValues,
			valuesOverwries,
		)
		if err != nil {
			return fmt.Errorf("creating render engine: %w", err)
		}

		if err := engine.RenderTargets(ctx, manifest, targets); err != nil {
			return fmt.Errorf("rendering targets: %w", err)
		}

		if err := engine.ResolveArtifacts(ctx); err != nil {
			return fmt.Errorf("resolving artifacts: %w", err)
		}

		if err := lockfile.Create(ctx, workDir); err != nil {
			return fmt.Errorf("creating lock file: %w", err)
		}

		log.Info().Int("count", len(targets)).Msg("successfully rendered all targets to work directory")

		if renderFlags.outPath == "" {
			// if no output is specified, we are done
			// this means the render command was most likely used for validation only
			log.Info().Msgf("no output path specified. render validation complete.")
			return nil
		}

		ext := filepath.Ext(renderFlags.outPath)
		if ext == "" {
			// if no extension is given, we assume the user wants a directory
			log.Info().Msgf("no archive extension specified, assuming directory output")

			// just move the directory
			if err := os.Rename(workDir, renderFlags.outPath); err != nil {
				return fmt.Errorf("moving output directory to %q: %w", renderFlags.outPath, err)
			}
			// prevent the deferred cleanup
			workDirMoved = true
			log.Info().Str("path", workDir).Msg("wrote rendered files to directory")
			return nil
		}

		compress := false
		switch {
		case strings.HasSuffix(renderFlags.outPath, ".tar.gz"):
			compress = true
		case strings.HasSuffix(renderFlags.outPath, ".tar"):
			compress = false
		default:
			return fmt.Errorf("unsupported archive extension %q (supported: .tar.gz)", ext)
		}

		if err := archive.Create(workDir, renderFlags.outPath, compress); err != nil {
			return fmt.Errorf("creating archive %q: %w", renderFlags.outPath, err)
		}
		log.Info().Str("path", renderFlags.outPath).Msg("wrote rendered files to archive")
		return nil
	},
}

func init() {
	rootCmd.AddCommand(renderCmd)

	renderCmd.Flags().StringVarP(&renderFlags.manifestPath, "manifest", "m", internal.ManifestFileName,
		"Path to the manifest file")
	renderCmd.Flags().StringSliceVarP(&renderFlags.valuesFiles, "values-from", "f", []string{},
		"Additional values files to merge, merged left to right")
	renderCmd.Flags().StringToStringVarP(&renderFlags.valueOverwrites, "values-overwrites", "v",
		make(map[string]string), "Additional values to overwrite. These have the highest precedence.")
	renderCmd.Flags().StringSliceVarP(&renderFlags.secretFiles, "secrets", "s", []string{},
		"Additional secrets files to merge, merged left to right")

	renderCmd.Flags().StringSliceVarP(&renderFlags.targets, "targets", "t", []string{},
		"List of targets to render (comma-separated)")
	renderCmd.Flags().StringSliceVarP(&renderFlags.tags, "tags", "", []string{},
		"List of tags to filter targets by (comma-separated)")
	renderCmd.Flags().BoolVarP(&renderFlags.allTargets, "all-targets", "A", false,
		"Render all targets defined in the manifest")

	// either -t <target> OR -t <target> --tag <tag> OR --tag <tag> OR -A
	renderCmd.MarkFlagsMutuallyExclusive("targets", "all-targets")
	renderCmd.MarkFlagsMutuallyExclusive("tags", "all-targets")
	renderCmd.MarkFlagsOneRequired("targets", "tags", "all-targets")

	renderCmd.Flags().StringVarP(&renderFlags.outPath, "out", "o", "",
		"Output path for rendered files")
}

func newStrategyRegistry() (*strategy.Registry, error) {
	return strategy.NewRegistry(
		// the fallback strategy: copy (or overwrite) files as-is
		&strategy.CopyOnlyStrategy{
			Overwrite: true,
		},
		map[string]strategy.FileStrategy{
			// *.properties files should be patched, not overwritten
			".properties": &strategy.PropertiesPatchStrategy{},
			".yml":        &strategy.YAMLPatchStrategy{},
			".yaml":       &strategy.YAMLPatchStrategy{},
			".json":       &strategy.JSONPatchStrategy{},
			".toml":       &strategy.TOMLPatchStrategy{},
		})
}

const (
	renderLongDescription = `The render command is the core of gok. It processes a manifest file (e.g., ` + internal.ManifestFileName + `)
to generate a final set of configuration files for one or more defined 'targets'.

A target represents a specific output, like a Minecraft server instance (e.g., 'proxy', 'survival').
Each target is built by layering 'templates' or 'overlays' in a specified order.

DIRECTORY STRUCTURE
--------- ---------
Gok expects a '` + internal.ManifestFileName + `' file as the entry point. The paths to templates and overlays
specified inside the manifest (using 'from:') are relative to the directory containing the
manifest file.

Example:

.
├── ` + internal.ManifestFileName + `
├── templates/
│   ├── paper/              (Base template for a Paper server)
│   │   └── ` + internal.TemplateManifestFileName + `
│   └── velocity/           (Base template for a Velocity proxy)
└── overlays/
    └── survival/           (Specific overrides for the 'survival' server)

TEMPLATES AND INHERITANCE
--------- --- -----------
A template is a directory of files. It can optionally contain a '` + internal.TemplateManifestFileName + `' file
to provide metadata (like a description) or to inherit from another template. Inheritance
allows you to build complex configurations from smaller, reusable components. For example,
a 'paper-velocity' template could inherit from a base 'paper' template.

FILE OPERATIONS
---- ----------
- Files ending in '.properties' are merged, not overwritten.
- Files with a '` + internal.TemplateInfix + `' extension (e.g., 'server` + internal.TemplateInfix + `.properties') are processed by the
  Go template engine before being written to their final destination (e.g., 'server.properties').
- A '` + internal.DeletionFileName + `' file within a template directory can be used to explicitly remove files
  that were added by a previously applied (e.g., inherited) template.

VALUE PRECEDENCE
----- ----------
Values are made available to Go template files ('` + internal.TemplateInfix + `'). They are merged with the following
order of precedence (later values override earlier ones):
1. Global values (from '` + internal.ManifestFileName + `')
2. External values (from files passed via --values / -f)
3. Target values (defined in 'targets.<target-id>.values')
4. Template-specific values (defined in 'targets.<target-id>.templates[n].values')
5. Values overwrites (from values passed via --values-overwrites)`

	renderExample = `
  # Render a single target
  gok render -t proxy

  # Render a target using an external values file for environment-specific config
  gok render -t survival -f survival-prod-values.yaml -o survival.tar.gz
  
  # Override values by specifying multiple files (last one wins)
  gok render -t proxy -f common.yaml -f dev.yaml`
)
