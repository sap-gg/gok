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

func TestPropertiesPatchStrategy(t *testing.T) {
	ctx := context.Background()

	baseProps := `
key.one=value1
key.two=old_value
`
	patchProps := `
key.two=new_value
key.three=value3
`

	t.Run("should merge properties into existing file", func(t *testing.T) {
		dstDir := t.TempDir()
		dstPath := filepath.Join(dstDir, "server.properties")
		require.NoError(t, os.WriteFile(dstPath, []byte(baseProps), 0644))

		strategy := &PropertiesPatchStrategy{}
		err := strategy.Apply(ctx, strings.NewReader(patchProps), dstPath)
		require.NoError(t, err)

		readBytes, err := os.ReadFile(dstPath)
		require.NoError(t, err)
		content := string(readBytes)

		// Properties order isn't guaranteed, so check for presence of lines
		assert.Contains(t, content, "key.one = value1")
		assert.Contains(t, content, "key.two = new_value")
		assert.Contains(t, content, "key.three = value3")
		assert.NotContains(t, content, "old_value")
	})
}
