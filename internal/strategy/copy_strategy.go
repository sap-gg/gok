package strategy

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/rs/zerolog/log"
)

var _ FileStrategy = (*CopyOnlyStrategy)(nil)

// CopyOnlyStrategy is a FileStrategy that simply copies files and overwrites them if Overwrite is true.
type CopyOnlyStrategy struct {
	Overwrite bool
}

func (s *CopyOnlyStrategy) Name() string {
	return "copy-only"
}

func (s *CopyOnlyStrategy) Apply(ctx context.Context, src, dst string, tr trackerApplier) error {
	log.Info().Msgf("[copy-only] copying %q to: %q...", filepath.Base(src), dst)

	// Best-effort context check, no I/O cancellation
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return fmt.Errorf("mkdir %q: %w", filepath.Dir(dst), err)
	}

	if _, err := os.Stat(dst); err == nil {
		if !s.Overwrite {
			log.Warn().
				Msgf("destination exists; skipping: %q", dst)
		}
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("stat dst %q: %w", dst, err)
	}

	sf, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("open src %q: %w", src, err)
	}
	defer sf.Close()

	df, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return fmt.Errorf("create dst %q: %w", dst, err)
	}
	defer df.Close()

	if _, err := io.Copy(df, sf); err != nil {
		return fmt.Errorf("copy to %q: %w", dst, err)
	}

	tr.Record(dst)
	return nil
}
