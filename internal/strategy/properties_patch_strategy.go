package strategy

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/magiconair/properties"
	"github.com/rs/zerolog/log"
)

var _ FileStrategy = (*PropertiesPatchStrategy)(nil)

type PropertiesPatchStrategy struct {
}

func (s *PropertiesPatchStrategy) Name() string {
	return "properties-patch"
}

func (s *PropertiesPatchStrategy) Apply(
	ctx context.Context,
	srcContent io.Reader,
	dst string,
) error {
	log.Info().Msgf("[properties-patch] merging into %q", dst)

	// Best-effort context check, no I/O cancellation
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	source, err := properties.LoadReader(srcContent, properties.UTF8)
	if err != nil {
		return fmt.Errorf("load source properties: %w", err)
	}

	// Ensure the destination directory exists
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return fmt.Errorf("mkdir for dst %q: %w", dst, err)
	}

	// Load target properties; it's okay if it doesn't exist
	target, err := properties.LoadFile(dst, properties.UTF8)
	if err != nil {
		if !os.IsNotExist(err) {
			return fmt.Errorf("load target properties file %q: %w", dst, err)
		}
		// If the file doesn't exist, start with an empty set
		target = properties.NewProperties()
	}

	// Merge the new properties into the existing ones
	target.Merge(source)

	// Write the merged properties back to the destination
	df, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return fmt.Errorf("create/truncate dst %q: %w", dst, err)
	}
	defer df.Close()

	if _, err := target.Write(df, properties.UTF8); err != nil {
		return fmt.Errorf("writing merged properties to %q: %w", dst, err)
	}

	return nil
}
