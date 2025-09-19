package cmd

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"

	"github.com/sap-gg/gok/internal"
	"github.com/sap-gg/gok/internal/archive"
	"github.com/sap-gg/gok/internal/diff"
)

var applyFlags = struct {
	destination string
	dryRun      bool
	force       bool
}{}

// applyCmd represents the apply command
var applyCmd = &cobra.Command{
	Use:     "apply",
	Short:   "Applies a rendered artifact to a destination directory.",
	Long:    applyLongDescription,
	Example: applyExample,
	Args:    cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		sourceArtifact := args[0]
		destinationDir := applyFlags.destination

		log.Info().Msgf("reading desired state from artifact: %s", sourceArtifact)
		desiredStateDir, err := os.MkdirTemp("", "gok-apply-desired-")
		if err != nil {
			return fmt.Errorf("create temp dir for desired state: %w", err)
		}
		defer os.RemoveAll(desiredStateDir)

		if err := archive.Extract(sourceArtifact, desiredStateDir); err != nil {
			return fmt.Errorf("extract artifact %q: %w", sourceArtifact, err)
		}

		// compare desired state with current state
		comparer := diff.NewComparer(destinationDir, desiredStateDir)
		report, err := comparer.Compare()
		if err != nil {
			return fmt.Errorf("compare desired and current state: %w", err)
		}

		// print the changes we are going to apply
		printDiffReport(report)

		if applyFlags.dryRun {
			log.Info().Msg("dry-run mode enabled, no changes will be applied")
			return nil
		}

		if report.HasConflicts() && !applyFlags.force {
			return fmt.Errorf("conflicts detected and --force not specified, aborting")
		}

		if !report.HasChanges() {
			log.Info().Msg("no changes detected, nothing to apply")
			return nil
		}

		log.Info().Msg("applying changes...")
		for _, path := range report.SortedPaths() {
			change := report.Changes[path]

			srcPath := filepath.Join(desiredStateDir, path)
			dstPath := filepath.Join(destinationDir, path)

			switch change.Type {
			case diff.Created, diff.Modified, diff.Conflict:
				log.Info().Str("path", path).Msg("copy/update")
				if err := copyFile(srcPath, dstPath); err != nil {
					return fmt.Errorf("failed to copy %s: %w", path, err)
				}
			case diff.Removed:
				log.Info().Str("path", path).Msg("remove")
				if err := os.Remove(dstPath); err != nil {
					if os.IsNotExist(err) {
						log.Warn().Msgf("file %s already removed", path)
						continue
					}
					return fmt.Errorf("failed to remove %s: %w", path, err)
				}
			default:
				// we don't care about unchanged files
			}
		}

		log.Info().Msg("updating lock file in destination")
		srcLockPath := filepath.Join(desiredStateDir, internal.LockFileName)
		dstLockPath := filepath.Join(destinationDir, internal.LockFileName)
		if err := copyFile(srcLockPath, dstLockPath); err != nil {
			return fmt.Errorf("failed to update lock file: %w", err)
		}

		log.Info().Msg("apply completed successfully")
		return nil
	},
}

func copyFile(srcPath, dstPath string) error {
	if err := os.MkdirAll(filepath.Dir(dstPath), 0755); err != nil {
		return fmt.Errorf("create parent directories for %q: %w", dstPath, err)
	}

	srcFile, err := os.Open(srcPath)
	if err != nil {
		return fmt.Errorf("open source file %q: %w", srcPath, err)
	}
	defer srcFile.Close()

	dstFile, err := os.Create(dstPath)
	if err != nil {
		return fmt.Errorf("create destination file %q: %w", dstPath, err)
	}
	defer dstFile.Close()

	_, err = io.Copy(dstFile, srcFile)
	return err
}

func init() {
	rootCmd.AddCommand(applyCmd)

	applyCmd.Flags().StringVarP(&applyFlags.destination, "destination", "d", "",
		"The destination directory to apply the artifact to. (required)")
	_ = applyCmd.MarkFlagRequired("destination")

	applyCmd.Flags().BoolVarP(&applyFlags.dryRun, "dry-run", "n", false,
		"Preview the changes without applying them.")

	applyCmd.Flags().BoolVarP(&applyFlags.force, "force", "f", false,
		"Force apply even if conflicts are detected.")
}

var (
	applyLongDescription = `The apply command takes a rendered artifact (a .tar.gz file) and applies its
contents to a specified output directory.

It performs the same comparison as 'gok diff' to ensure safety.
The command will only create, update or delete files as necessary.

SAFETY
------
By default, 'gok apply' will abort if it detects that files in the destination
directory have been modified externally (a 'conflict'). To proceed and
overwrite these manual changes, you can use the '--force' flag.
`

	applyExample = `
# Preview the changes that would be applied to the server directory
gok apply ./new-build.tar.gz --destination /opt/server --dry-run

# Apply the artifact. This will fail if conflicts are detected.
gok apply ./new-build.tar.gz --destination /opt/server

# Apply the artifact and overwrite any conflicting files.
gok apply ./new-build.tar.gz --destination /opt/server --force`
)
