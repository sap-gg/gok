package strategy

import (
	"context"
	"fmt"
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

func (s *PropertiesPatchStrategy) Apply(ctx context.Context, src, dst string, tr trackerApplier) error {
	log.Info().Msgf("[properties-patch] merging %q into %q", filepath.Base(src), dst)

	// Best-effort context check, no I/O cancellation
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	source, err := properties.LoadFile(src, properties.UTF8)
	if err != nil {
		return fmt.Errorf("load source properties file: %w", err)
	}

	target, err := properties.LoadFile(dst, properties.UTF8)
	if err != nil {
		return fmt.Errorf("load target properties file: %w", err)
	}

	target.Merge(source)

	df, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return fmt.Errorf("create dst %q: %w", dst, err)
	}
	defer df.Close()

	if _, err := target.Write(df, properties.UTF8); err != nil {
		return fmt.Errorf("merging properties: %w", err)
	}

	return nil
}
