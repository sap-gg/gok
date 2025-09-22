package strategy

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTOMLPatchStrategy(t *testing.T) {
	ctx := context.Background()

	baseTOML := `
[server]
host = "localhost"
port = 8080

[features]
feature_a = true
`
	patchTOML := `
[server]
port = 9090 # Overwrite
user = "admin"  # Add

[features]
feature_b = true # Add
`
	t.Run("should merge patch into existing TOML", func(t *testing.T) {
		dstDir := t.TempDir()
		dstPath := filepath.Join(dstDir, "config.toml")
		require.NoError(t, os.WriteFile(dstPath, []byte(baseTOML), 0644))

		strategy := &TOMLPatchStrategy{}
		err := strategy.Apply(ctx, strings.NewReader(patchTOML), dstPath)
		require.NoError(t, err)

		// Assert the final file has the merged content
		readBytes, err := os.ReadFile(dstPath)
		require.NoError(t, err)

		content := string(readBytes)
		assert.Contains(t, content, `host = 'localhost'`) // Unchanged
		assert.Contains(t, content, `port = 9090`)        // Overwritten
		assert.Contains(t, content, `user = 'admin'`)     // Added
		assert.Contains(t, content, `feature_a = true`)   // Unchanged
		assert.Contains(t, content, `feature_b = true`)   // Added
	})

	t.Run("should create new TOML if destination does not exist", func(t *testing.T) {
		dstDir := t.TempDir()
		dstPath := filepath.Join(dstDir, "new_config.toml")

		strategy := &TOMLPatchStrategy{}
		err := strategy.Apply(ctx, strings.NewReader(patchTOML), dstPath)
		require.NoError(t, err)

		// Assert file was created with the patch content
		readBytes, err := os.ReadFile(dstPath)
		require.NoError(t, err)
		assert.Contains(t, string(readBytes), `port = 9090`)
	})
}
