package artifact

import (
	"context"
	"fmt"
	"io"

	"github.com/rs/zerolog/log"

	"github.com/sap-gg/gok/internal"
)

// Tracker collects artifact definitions during the render pass and orchestrates their processing.
type Tracker struct {
	artifacts map[string]*Spec
	processor *Processor
}

// NewTracker creates a new artifact Tracker.
func NewTracker() (*Tracker, error) {
	processor, err := NewProcessor()
	if err != nil {
		return nil, err
	}
	return &Tracker{
		artifacts: make(map[string]*Spec),
		processor: processor,
	}, nil
}

// Register parses an artifact manifest and stores it for later processing.
func (t *Tracker) Register(outputPath string, reader io.Reader) error {
	var spec Spec
	if err := internal.NewYAMLDecoder(reader).Decode(&spec); err != nil {
		if internal.IsDecodeErrorAndPrint(err) {
			return fmt.Errorf("decoding artifact spec")
		}
		return fmt.Errorf("decoding artifact spec: %w", err)
	}
	if err := spec.Validate(); err != nil {
		return fmt.Errorf("validating artifact spec: %w", err)
	}

	log.Debug().
		Str("path", outputPath).
		Msg("registering artifact for resolution")
	t.artifacts[outputPath] = &spec
	return nil
}

func (t *Tracker) ProcessAll(ctx context.Context) error {
	if len(t.artifacts) == 0 {
		log.Debug().Msg("no artifacts to process")
		return nil
	}

	for path, spec := range t.artifacts {
		log.Info().
			Str("path", path).
			Str("url", spec.Source.HTTP.URL).
			Msg("processing artifact")

		if err := t.processor.Process(ctx, path, spec); err != nil {
			return fmt.Errorf("processing artifact %q: %w", path, err)
		}
	}

	log.Info().Msg("all artifacts processed successfully")
	return nil
}
