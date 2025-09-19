package cmd

import (
	"fmt"
	"os"

	"github.com/fatih/color"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"

	"github.com/sap-gg/gok/internal"
	"github.com/sap-gg/gok/internal/archive"
	"github.com/sap-gg/gok/internal/diff"
)

// diffCmd represents the diff command.
// It's very similar to the applyCmd (with dry run always enabled),
// but it does not make any changes to the output directory.
var diffCmd = &cobra.Command{
	Use:     "diff <source-artifact.tar.gz> <output-dir>",
	Short:   "Compares a rendered artifact with an existing output directory.",
	Long:    diffLongDescription,
	Example: diffExample,
	Args:    cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		sourceArtifact := args[0]
		currentOutputDir := args[1]

		log.Info().Msgf("reading desired state from artifact %s", sourceArtifact)
		tempDir, err := os.MkdirTemp("", "gok-diff-desired-")
		if err != nil {
			return fmt.Errorf("creating temp dir: %w", err)
		}
		defer os.RemoveAll(tempDir)

		if err := archive.Extract(sourceArtifact, tempDir); err != nil {
			return fmt.Errorf("extracting source artifact: %w", err)
		}

		comparer := diff.NewComparer(currentOutputDir, tempDir)
		report, err := comparer.Compare()
		if err != nil {
			return fmt.Errorf("comparing states: %w", err)
		}

		printDiffReport(report)

		if report.HasConflicts() {
			log.Warn().Msg("conflicts detected. Please resolve them before applying changes.")
			return fmt.Errorf("diff completed with conflicts")
		}
		if report.HasChanges() {
			log.Info().Msg("changes detected. You can proceed with 'gok apply' to apply them.")
		} else {
			log.Info().Msg("no changes detected. Current state matches desired state.")
		}

		return nil
	},
}

func printDiffReport(report *diff.Report) {
	if !report.HasChanges() {
		return
	}

	for _, path := range report.SortedPaths() {
		change := report.Changes[path]
		switch change.Type {
		case diff.Created:
			color.Green("+ %s", path)
		case diff.Modified:
			color.Yellow("~ %s", path)
		case diff.Removed:
			color.Red("- %s", path)
		case diff.Conflict:
			color.HiRed("! %s (conflict)", path)
		case diff.Unchanged:
			// do nothing
		}
	}
}

func init() {
	rootCmd.AddCommand(diffCmd)
}

const (
	diffLongDescription = `The diff command provides a safe, read-only preview of the changes that would be
made by applying a rendered artifact. It is similar to the 'apply' command,
but it does not modify any files in the output directory.

It performs a three-way comparison between:
1. The 'desired state' (the contents of the <source-artifact.tar.gz> file)
2. The 'last known state' (from the` + internal.LockFileName + ` file in the <output-dir>)
3. The 'actual current state' (the real files on the disk in the <output-dir>

This allows it to detect not only pending changes but also 'conflicts' or 'drift',
which occur when files have been modified on the target outside of the gok workflow.`

	diffExample = `
# Compare the newly rendered artifact with the current server state
gok diff ./new-build.tar.gz /opt/minecraft/server'`
)
