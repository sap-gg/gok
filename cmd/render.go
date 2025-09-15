package cmd

import (
	"errors"
	"fmt"
	"os"

	"github.com/goccy/go-yaml"
	"github.com/rs/zerolog/log"
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
	Use:   "render",
	Short: "Render one or more targets from a manifest into a temporary work tree.",
	Example: `
  # Render a single target
  gok render -m manifest.yaml -t proxy-1

  # Render multiple targets
  gok render -m manifest.yaml -t proxy-1 -t proxy-2

  # Render all targets
  gok render -m manifest.yaml -A
`,
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
		} else {
			workDir, err := os.MkdirTemp("", "gok-workdir-")
			if err != nil {
				return fmt.Errorf("creating working directory: %w", err)
			}
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

	renderCmd.Flags().StringVarP(&renderFlags.manifestPath, "manifest", "m", "",
		"Path to the manifest file")
	_ = renderCmd.MarkFlagRequired("manifest")

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
