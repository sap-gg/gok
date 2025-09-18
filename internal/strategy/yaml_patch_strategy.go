package strategy

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/goccy/go-yaml"
	"github.com/rs/zerolog/log"

	"github.com/sap-gg/gok/internal/merge"
)

var _ FileStrategy = (*YAMLPatchStrategy)(nil)

// YAMLPatchStrategy is a file strategy that applies YAML patches to files.
type YAMLPatchStrategy struct{}

// Name returns the name of the strategy.
func (s *YAMLPatchStrategy) Name() string {
	return "yaml-patch"
}

// Apply applies the YAML patch strategy to the given file content.
// It expects the content to be a valid YAML document and applies the patch accordingly.
func (s *YAMLPatchStrategy) Apply(
	ctx context.Context,
	srcContent io.Reader,
	dst string,
) error {
	log.Info().Msgf("[yaml-patch] applying to %q", dst)

	sourceBytes, err := io.ReadAll(srcContent)
	if err != nil {
		return fmt.Errorf("read source content: %w", err)
	}

	var sourceData map[string]any
	if err := yaml.Unmarshal(sourceBytes, &sourceData); err != nil {
		return fmt.Errorf("unmarshal source YAML for %q: %w", dst, err)
	}

	// Ensure the destination directory exists
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return fmt.Errorf("mkdir for dst %q: %w", dst, err)
	}

	var targetData map[string]any
	targetBytes, err := os.ReadFile(dst)
	if err != nil {
		if !os.IsNotExist(err) {
			return fmt.Errorf("read target YAML %q: %w", dst, err)
		}
		// If the file doesn't exist, start with an empty map
		targetData = make(map[string]any)
	} else {
		if err := yaml.Unmarshal(targetBytes, &targetData); err != nil {
			return fmt.Errorf("unmarshal target YAML %q: %w", dst, err)
		}
	}

	mergedData := merge.DeepMergeMaps(targetData, sourceData)

	// Write the merged properties back to the destination
	df, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return fmt.Errorf("create/truncate dst %q: %w", dst, err)
	}
	defer df.Close()

	if err := yaml.NewEncoder(df).EncodeContext(ctx, mergedData); err != nil {
		return fmt.Errorf("writing merged YAML to %q: %w", dst, err)
	}

	return nil
}
