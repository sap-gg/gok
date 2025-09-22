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

func TestCopyOnlyStrategy(t *testing.T) {
	ctx := context.Background()
	content := "hello world"
	srcReader := strings.NewReader(content)

	t.Run("should copy file to new destination", func(t *testing.T) {
		dstDir := t.TempDir()
		dstPath := filepath.Join(dstDir, "output.txt")

		strategy := &CopyOnlyStrategy{Overwrite: false}
		err := strategy.Apply(ctx, srcReader, dstPath)
		require.NoError(t, err)

		readBytes, err := os.ReadFile(dstPath)
		require.NoError(t, err)
		assert.Equal(t, content, string(readBytes))
	})

	t.Run("should not overwrite existing file by default", func(t *testing.T) {
		dstDir := t.TempDir()
		dstPath := filepath.Join(dstDir, "output.txt")
		existingContent := "i already exist"
		require.NoError(t, os.WriteFile(dstPath, []byte(existingContent), 0644))

		strategy := &CopyOnlyStrategy{Overwrite: false}
		err := strategy.Apply(ctx, srcReader, dstPath)
		require.NoError(t, err)

		// Assert file content has NOT changed
		readBytes, err := os.ReadFile(dstPath)
		require.NoError(t, err)
		assert.Equal(t, existingContent, string(readBytes))
	})

	t.Run("should overwrite existing file when Overwrite is true", func(t *testing.T) {
		dstDir := t.TempDir()
		dstPath := filepath.Join(dstDir, "output.txt")
		existingContent := "i already exist"
		require.NoError(t, os.WriteFile(dstPath, []byte(existingContent), 0644))

		_, _ = srcReader.Seek(0, 0) // Reset reader
		strategy := &CopyOnlyStrategy{Overwrite: true}
		err := strategy.Apply(ctx, srcReader, dstPath)
		require.NoError(t, err)

		// Assert file content HAS changed
		readBytes, err := os.ReadFile(dstPath)
		require.NoError(t, err)
		assert.Equal(t, content, string(readBytes))
	})
}
