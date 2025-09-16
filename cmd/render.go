package cmd

import (
	"errors"
	"fmt"
	"os"

	"github.com/goccy/go-yaml"
	"github.com/rs/zerolog/log"
	"github.com/sap-gg/gok/internal"
	"github.com/sap-gg/gok/internal/render"
	"github.com/sap-gg/gok/internal/strategy"
	"github.com/spf13/cobra"
)

var renderFlags = struct {
	manifestPath string
	targets      []string
	tags         []string
	allTargets   bool
	outPath      string
	noDelete     bool
	overwrite    bool
}{}

// renderCmd represents the render command
var renderCmd = &cobra.Command{
	Use:   "render -m <manifest> -t <target> ...",
	Short: "Renders targets from a manifest by applying a series of templates and overlays.",

	Long: `The render command is the core of gok. It processes a manifest file (e.g., ` + internal.ManifestFileName + `)
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
- Files are copied by default. If a destination file already exists, it is skipped unless
  '--overwrite' is used.
- Files ending in '.properties' are merged, not overwritten.
- Files with a '` + internal.TemplateInfix + `' extension (e.g., 'server` + internal.TemplateInfix + `.properties') are processed by the
  Go template engine before being written to their final destination (e.g., 'server.properties').
- A '` + internal.DeletionFileName + `' file within a template directory can be used to explicitly remove files
  that were added by a previously applied (e.g., inherited) template.

VALUE PRECEDENCE
----- ----------
Values are made available to Go template files ('` + internal.TemplateInfix + `'). They are merged with the following
order of precedence (later values override earlier ones):
1. Global values (defined in 'globals.values' in the manifest)
2. Target values (defined in 'targets.<target-id>.values')
3. Template-specific values (defined in 'targets.<target-id>.templates[n].values')`,

	Example: `
  # Render a single target named 'proxy' from the manifest
  gok render -m ` + internal.ManifestFileName + ` -t proxy

  # Render multiple targets by name
  gok render -m ` + internal.ManifestFileName + ` -t proxy -t survival

  # Render all targets that have the 'production' tag
  gok render -m ` + internal.ManifestFileName + ` --tags production

  # Render all targets defined in the manifest
  gok render -m ` + internal.ManifestFileName + ` -A

  # Render to a specific output directory, overwriting it if it exists
  gok render -m ` + internal.ManifestFileName + ` -A -o ./my-server-output --overwrite

  # Render and keep the temporary directory for debugging
  gok render -m ` + internal.ManifestFileName + ` -t proxy --no-delete`,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()

		manifest, manifestDir, err := render.ReadManifest(ctx, renderFlags.manifestPath)
		if err != nil {
			var yamlError yaml.Error
			if errors.As(err, &yamlError) {
				fmt.Println(yamlError.FormatError(true, true))
				return fmt.Errorf("parsing manifest")
			}
			return fmt.Errorf("reading manifest: %w", err)
		}

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

		var workDir string
		if renderFlags.outPath != "" {
			workDir = renderFlags.outPath

			// check that it does not yet exist
			if _, err := os.Stat(workDir); err == nil {
				return fmt.Errorf("output path %q already exists", workDir)
			}

			log.Debug().Str("dir", workDir).Msg("using specified output directory")
		} else {
			tmp, err := os.MkdirTemp("", "gok-workdir-")
			if err != nil {
				return fmt.Errorf("creating working directory: %w", err)
			}
			workDir = tmp

			log.Debug().Str("dir", workDir).Msg("created temporary directory")
		}

		if !renderFlags.noDelete {
			defer func() {
				if rmErr := os.RemoveAll(workDir); rmErr != nil {
					log.Debug().Err(rmErr).Msg("failed to remove temporary directory")
				} else {
					log.Info().Str("dir", workDir).Msg("removed temporary directory")
				}
			}()
		}

		registry, err := strategy.NewRegistry(&strategy.CopyOnlyStrategy{
			Overwrite: renderFlags.overwrite,
		}, map[string]strategy.FileStrategy{
			// *.properties files should be patched, not overwritten
			".properties": &strategy.PropertiesPatchStrategy{},
		})
		if err != nil {
			return fmt.Errorf("creating strategy registry: %w", err)
		}

		engine, err := render.NewEngine(manifestDir, workDir, registry)
		if err != nil {
			return fmt.Errorf("creating render engine: %w", err)
		}

		if err := engine.RenderTargets(ctx, manifest, targets); err != nil {
			return fmt.Errorf("rendering targets: %w", err)
		}

		log.Info().Int("count", len(targets)).Msg("rendered targets")
		return nil
	},
}

func init() {
	rootCmd.AddCommand(renderCmd)

	renderCmd.Flags().StringVarP(&renderFlags.manifestPath, "manifest", "m", internal.ManifestFileName,
		"Path to the manifest file")

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
		"Output path for rendered files (defaults to target-specific paths)")
	renderCmd.Flags().BoolVar(&renderFlags.noDelete, "no-delete", false,
		"Do not delete the temporary working directory (for debugging purposes)")

	renderCmd.Flags().BoolVar(&renderFlags.overwrite, "overwrite", false,
		"Overwrite existing files in the output directory")
}
