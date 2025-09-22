package artifact

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupTestServer(t *testing.T, content string) (*httptest.Server, string) {
	hasher := sha256.New()
	hasher.Write([]byte(content))
	hash := hex.EncodeToString(hasher.Sum(nil))

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Logf("Mock server received a request for: %s", r.URL.Path)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(content))
	}))

	t.Cleanup(server.Close)
	return server, hash
}

func TestProcessor_Integration(t *testing.T) {
	ctx := context.Background()

	fileContent := "This is the content of our test artifact."
	badFileContent := "This content is intentionally wrong."

	server, correctHash := setupTestServer(t, fileContent)
	badServer, _ := setupTestServer(t, badFileContent)

	cacheDir := t.TempDir()
	outputDir := t.TempDir()

	processor := &Processor{
		cacheDir: cacheDir,
	}

	spec := &Spec{
		Version:   SpecVersion,
		Algorithm: "sha256",
		Checksum:  correctHash,
		Source: Source{
			HTTP: &HTTPSource{
				URL: server.URL,
			},
		},
	}

	outputPath := filepath.Join(outputDir, "my-artifact.txt")

	// Scenario 1: Cache Miss
	t.Run("Cache Miss - should download and cache the file", func(t *testing.T) {
		err := processor.Process(ctx, outputPath, spec)
		require.NoError(t, err)

		assert.FileExists(t, outputPath)
		content, err := os.ReadFile(outputPath)
		require.NoError(t, err)
		assert.Equal(t, fileContent, string(content))

		// 2. Check that the file now exists in the cache
		cachePath := filepath.Join(cacheDir, spec.Algorithm, spec.Checksum)
		assert.FileExists(t, cachePath)
	})

	// Scenario 2: Cache Hit
	t.Run("Cache Hit - should use cached file without downloading", func(t *testing.T) {
		newOutputPath := filepath.Join(outputDir, "my-artifact-copy.txt")

		failingServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			t.Fatal("Server should not have been called on a cache hit!")
		}))
		defer failingServer.Close()
		spec.Source.HTTP.URL = failingServer.URL

		err := processor.Process(ctx, newOutputPath, spec)
		require.NoError(t, err)

		assert.FileExists(t, newOutputPath)
		content, err := os.ReadFile(newOutputPath)
		require.NoError(t, err)
		assert.Equal(t, fileContent, string(content))
	})

	// Scenario 3: Checksum Mismatch
	t.Run("Checksum Mismatch - should fail with an error", func(t *testing.T) {
		require.NoError(t, os.RemoveAll(cacheDir))
		require.NoError(t, os.MkdirAll(cacheDir, 0755))

		badSpec := &Spec{
			Version:   SpecVersion,
			Algorithm: "sha256",
			Checksum:  correctHash, // Expecting the GOOD hash
			Source: Source{
				HTTP: &HTTPSource{
					URL: badServer.URL, // But downloading from the BAD server
				},
			},
		}
		mismatchOutputPath := filepath.Join(outputDir, "mismatch-artifact.txt")

		err := processor.Process(ctx, mismatchOutputPath, badSpec)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "checksum mismatch")
		t.Logf("Received expected error: %v", err)

		assert.NoFileExists(t, mismatchOutputPath)
		cachePath := filepath.Join(cacheDir, badSpec.Algorithm, badSpec.Checksum)
		assert.NoFileExists(t, cachePath)
	})
}
