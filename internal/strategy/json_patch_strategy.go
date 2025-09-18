package strategy

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/rs/zerolog/log"

	"github.com/sap-gg/gok/internal/merge"
)

var _ FileStrategy = (*JSONPatchStrategy)(nil)

// JSONPatchStrategy is a file strategy that applies JSON patches to files.
type JSONPatchStrategy struct{}

// Name returns the name of the strategy.
func (s *JSONPatchStrategy) Name() string {
	return "json-patch"
}

// Apply applies the JSON patch strategy to the given file content.
// It expects the content to be a valid JSON document and applies the patch accordingly.
func (s *JSONPatchStrategy) Apply(
	_ context.Context,
	srcContent io.Reader,
	dst string,
) error {
	log.Info().Msgf("[json-patch] applying to %q", dst)

	var sourceData map[string]any
	if err := json.NewDecoder(srcContent).Decode(&sourceData); err != nil {
		return fmt.Errorf("unmarshal source JSON for %q: %w", dst, err)
	}

	// Ensure the destination directory exists
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return fmt.Errorf("mkdir for dst %q: %w", dst, err)
	}

	var targetData map[string]any
	targetBytes, err := os.ReadFile(dst)
	if err != nil {
		if !os.IsNotExist(err) {
			return fmt.Errorf("open target JSON %q: %w", dst, err)
		}
		// If the file doesn't exist, start with an empty map
		targetData = make(map[string]any)
	} else {
		if err := json.Unmarshal(targetBytes, &targetData); err != nil {
			return fmt.Errorf("unmarshal target JSON %q: %w", dst, err)
		}
	}

	mergedData := merge.DeepMergeMaps(targetData, sourceData)

	// Write the merged properties back to the destination
	df, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return fmt.Errorf("create/truncate dst %q: %w", dst, err)
	}
	defer df.Close()

	if err := json.NewEncoder(df).Encode(mergedData); err != nil {
		return fmt.Errorf("writing merged JSON to %q: %w", dst, err)
	}

	return nil

}
