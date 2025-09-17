package strategy

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/pelletier/go-toml/v2"
	"github.com/rs/zerolog/log"

	"github.com/sap-gg/gok/internal/merge"
)

var _ FileStrategy = (*TOMLPatchStrategy)(nil)

// TOMLPatchStrategy is a file strategy that applies TOML patches to files.
type TOMLPatchStrategy struct{}

// Name returns the name of the strategy.
func (s *TOMLPatchStrategy) Name() string {
	return "toml-patch"
}

// Apply applies the TOML patch strategy to the given file content.
// It expects the content to be a valid TOML document and applies the patch accordingly.
func (s *TOMLPatchStrategy) Apply(
	ctx context.Context,
	srcContent io.Reader,
	dst string,
	tr trackerApplier,
) error {
	log.Info().Msgf("[toml-patch] applying to %q", dst)

	var sourceData map[string]any
	if err := toml.NewDecoder(srcContent).Decode(&sourceData); err != nil {
		return fmt.Errorf("unmarshal source TOML for %q: %w", dst, err)
	}

	// Ensure the destination directory exists
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return fmt.Errorf("mkdir for dst %q: %w", dst, err)
	}

	var targetData map[string]any
	targetBytes, err := os.ReadFile(dst)
	if err != nil {
		if !os.IsNotExist(err) {
			return fmt.Errorf("open target TOML %q: %w", dst, err)
		}
		// If the file doesn't exist, start with an empty map
		targetData = make(map[string]any)
	} else {
		if err := toml.Unmarshal(targetBytes, &targetData); err != nil {
			return fmt.Errorf("unmarshal target TOML %q: %w", dst, err)
		}
	}

	mergedData := merge.DeepMergeMaps(targetData, sourceData)

	// Write the merged properties back to the destination
	df, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return fmt.Errorf("create/truncate dst %q: %w", dst, err)
	}
	defer df.Close()

	if err := toml.NewEncoder(df).Encode(mergedData); err != nil {
		return fmt.Errorf("writing merged properties to %q: %w", dst, err)
	}

	tr.Record(dst)
	return nil
}
