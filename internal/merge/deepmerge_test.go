package merge

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type values = map[string]any

func TestDeepMergeMaps(t *testing.T) {
	t.Run("Simple merge should combine keys", func(t *testing.T) {
		map1 := values{"a": 1}
		map2 := values{"b": 2}
		merged := DeepMergeMaps(map1, map2)
		expected := values{"a": 1, "b": 2}
		assert.Equal(t, expected, merged)
	})

	t.Run("Later maps should overwrite earlier maps", func(t *testing.T) {
		map1 := values{"a": 1, "b": "old"}
		map2 := values{"b": "new", "c": 3}
		merged := DeepMergeMaps(map1, map2)
		expected := values{"a": 1, "b": "new", "c": 3}
		assert.Equal(t, expected, merged)
	})

	t.Run("Nested maps should be merged recursively", func(t *testing.T) {
		map1 := values{
			"server": values{
				"host": "localhost",
				"port": 8080,
			},
		}
		map2 := values{
			"server": values{
				"port": 9090,    // Overwrite
				"user": "admin", // Add
			},
		}
		merged := DeepMergeMaps(map1, map2)
		expected := values{
			"server": values{
				"host": "localhost",
				"port": 9090,
				"user": "admin",
			},
		}
		assert.Equal(t, expected, merged)
	})

	t.Run("Merging should not modify original maps", func(t *testing.T) {
		original := values{
			"a": values{
				"b": 1,
			},
		}
		overwrite := values{
			"a": values{
				"c": 2,
			},
		}
		_ = DeepMergeMaps(original, overwrite)
		// Check that the original map was not mutated
		require.Equal(t, 1, len(original["a"].(values)))
		assert.Equal(t, 1, original["a"].(values)["b"])
	})
}
