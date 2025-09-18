package strategy

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"strings"
)

// FileStrategy defines how to apply source content onto a destination path.
type FileStrategy interface {
	// Name returns a human-friendly strategy name for logging/metrics.
	Name() string

	// Apply takes content from the srcContent reader, applies it to the dst path,
	// and reports whether dst was created or modified via the tracker.
	Apply(ctx context.Context, srcContent io.Reader, dst string) error
}

// Registry maps file extensions to strategies.
type Registry struct {
	byExtension map[string]FileStrategy
	// fallback is used if no strategy matches the file extension.
	fallback FileStrategy
}

// NewRegistry constructs a registry.
func NewRegistry(fallback FileStrategy, mappings map[string]FileStrategy) (*Registry, error) {
	if fallback == nil {
		return nil, fmt.Errorf("fallback strategy cannot be nil")
	}
	byExt := make(map[string]FileStrategy)
	for ext, s := range mappings {
		ext = strings.ToLower(strings.TrimSpace(ext))
		if ext == "" || !strings.HasPrefix(ext, ".") {
			return nil, fmt.Errorf("invalid extension key for strategy: %q", ext)
		}
		byExt[ext] = s
	}
	return &Registry{
		byExtension: byExt,
		fallback:    fallback,
	}, nil
}

// For returns the strategy for a given filename.
func (r *Registry) For(filename string) (FileStrategy, bool) {
	ext := strings.ToLower(filepath.Ext(filename))
	if s, ok := r.byExtension[ext]; ok {
		return s, true
	}
	return r.fallback, false
}

// Fallback returns the fallback strategy.
func (r *Registry) Fallback() FileStrategy {
	return r.fallback
}
