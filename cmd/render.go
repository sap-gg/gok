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
	allTargets   bool
	outPath      string
	noDelete     bool
	overwrite    bool
}{}

// renderCmd represents the render command
var renderCmd = &cobra.Command{
	Use: "render",
	// TODO: add short and long descriptions
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

		targets, err := render.SelectTargets(manifest, renderFlags.allTargets, renderFlags.targets)
		if err != nil {
			return fmt.Errorf("selecting targets: %w", err)
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
		}, map[string]strategy.FileStrategy{})
		if err != nil {
			return fmt.Errorf("creating strategy registry: %w", err)
		}

		engine, err := render.NewEngine(manifestDir, workDir, registry)
		if err != nil {
			return fmt.Errorf("creating render engine: %w", err)
		}

		if err := engine.RenderTargets(ctx, targets); err != nil {
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
	renderCmd.Flags().BoolVarP(&renderFlags.allTargets, "all-targets", "A", false,
		"Render all targets defined in the manifest")
	renderCmd.MarkFlagsMutuallyExclusive("targets", "all-targets")
	renderCmd.MarkFlagsOneRequired("targets", "all-targets")

	renderCmd.Flags().StringVarP(&renderFlags.outPath, "out", "o", "",
		"Output path for rendered files (defaults to target-specific paths)")
	renderCmd.Flags().BoolVar(&renderFlags.noDelete, "no-delete", false,
		"Do not delete the temporary working directory (for debugging purposes)")

	renderCmd.Flags().BoolVar(&renderFlags.overwrite, "overwrite", false,
		"Overwrite existing files in the output directory")
}
