package artifact

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"

	"github.com/rs/zerolog/log"
)

// Processor handles the fetching, verification and caching of a single artifact.
type Processor struct {
	cacheDir string
}

// NewProcessor creates a new Processor with the given cache directory.
func NewProcessor() (*Processor, error) {
	cacheDir, err := os.UserCacheDir()
	if err != nil {
		return nil, fmt.Errorf("determining user cache directory: %w", err)
	}
	gokCacheDir := filepath.Join(cacheDir, "gok", "artifacts")
	if err := os.MkdirAll(gokCacheDir, 0o755); err != nil {
		return nil, fmt.Errorf("creating cache directory: %w", err)
	}
	return &Processor{cacheDir: gokCacheDir}, nil
}

// Process ensures the artifact is present at the given outputPath using the cache.
func (p *Processor) Process(ctx context.Context, outputPath string, spec *Spec) error {
	cachePath := filepath.Join(p.cacheDir, spec.Algorithm, spec.Checksum)

	// first check if the artifact is already in the cache
	if _, err := os.Stat(cachePath); err == nil {
		log.Info().
			Str("path", cachePath).
			Msg("artifact found in cache")
		return p.placeFile(cachePath, outputPath)
	}

	// cache miss: download / fetch the artifact
	log.Info().
		Str("path", cachePath).
		Str("url", spec.Source.HTTP.URL).
		Msg("artifact not found in cache, downloading")
	if err := p.download(ctx, cachePath, spec); err != nil {
		return err
	}

	// place the newly downloaded file
	return p.placeFile(cachePath, outputPath)
}

func (p *Processor) placeFile(cachePath, destPath string) error {
	src, err := os.Open(cachePath)
	if err != nil {
		return fmt.Errorf("opening cached artifact: %w", err)
	}
	defer src.Close()

	if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
		return fmt.Errorf("creating destination directory: %w", err)
	}

	dst, err := os.Create(destPath)
	if err != nil {
		return fmt.Errorf("creating destination file: %w", err)
	}
	defer dst.Close()

	_, err = io.Copy(dst, src)
	if err != nil {
		return fmt.Errorf("copying artifact to destination: %w", err)
	}

	log.Info().
		Str("path", destPath).
		Msg("artifact placed at destination")
	return nil
}

func (p *Processor) download(ctx context.Context, cachePath string, spec *Spec) error {
	tmpFile, err := os.CreateTemp(p.cacheDir, "download-*")
	if err != nil {
		return fmt.Errorf("creating temp file for download: %w", err)
	}
	defer os.Remove(tmpFile.Name()) // clean up if it didn't get moved
	defer tmpFile.Close()

	req, err := http.NewRequestWithContext(ctx, "GET", spec.Source.HTTP.URL, nil)
	if err != nil {
		return fmt.Errorf("creating http request: %w", err)
	}
	for k, v := range spec.Source.HTTP.Headers {
		req.Header.Set(k, v)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("performing http request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected http status: %s", resp.Status)
	}

	// download and verify checksum on the fly
	hasher := sha256.New()
	multiWriter := io.MultiWriter(tmpFile, hasher)

	if _, err := io.Copy(multiWriter, resp.Body); err != nil {
		return fmt.Errorf("downloading artifact: %w", err)
	}

	actualChecksum := hex.EncodeToString(hasher.Sum(nil))
	if actualChecksum != spec.Checksum {
		return fmt.Errorf("checksum mismatch: expected %s, got %s", spec.Checksum, actualChecksum)
	}

	// move the temp file to the cache path
	if err := os.MkdirAll(filepath.Dir(cachePath), 0o755); err != nil {
		return fmt.Errorf("creating cache directory: %w", err)
	}
	if err := os.Rename(tmpFile.Name(), cachePath); err != nil {
		return fmt.Errorf("moving file to cache: %w", err)
	}

	log.Info().
		Str("path", cachePath).
		Msg("artifact downloaded and cached")
	return nil
}
