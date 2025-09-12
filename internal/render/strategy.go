package render

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
)

// FileStrategy defines how to apply a single source file onto a destination path.
// TODO(future): implement Merge/Overwrite policies for YAML/JSON/Properties.
type FileStrategy interface {
	// Name returns a human-friendly strategy name for logging/metrics.
	Name() string

	// Apply copies/merges src -> dst and reports whether dst was created or modified.
	// MUST record changes via Tracker to generate the lock-file later.
	Apply(ctx context.Context, src, dst string, tr *Tracker) error
}

// StrategyRegistry maps file extensions to strategies.
type StrategyRegistry struct {
	byExtension map[string]FileStrategy
	// fallback is used if no strategy matches the file extension.
	fallback FileStrategy
}

// NewStrategyRegistry constructs a registry.
func NewStrategyRegistry(fallback FileStrategy, mappings map[string]FileStrategy) (*StrategyRegistry, error) {
	byExt := make(map[string]FileStrategy)
	for ext, s := range mappings {
		ext = strings.ToLower(strings.TrimSpace(ext))
		if ext == "" || !strings.HasPrefix(ext, ".") {
			return nil, fmt.Errorf("invalid extension key for strategy: %q", ext)
		}
		byExt[ext] = s
	}
	return &StrategyRegistry{
		byExtension: byExt,
		fallback:    fallback,
	}, nil
}

// For returns the strategy for a given filename.
func (r *StrategyRegistry) For(filename string) FileStrategy {
	ext := strings.ToLower(filepath.Ext(filename))
	if s, ok := r.byExtension[ext]; ok {
		return s
	}
	return r.fallback
}
