package render

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLookupNestedValue(t *testing.T) {
	data := Values{
		"top_level": "value",
		"database": Values{
			"host": "localhost",
			"port": 5432,
			"connection": Values{
				"pool_size": 10,
			},
		},
		"a": Values{
			"b": "not a map",
		},
	}

	testCases := []struct {
		name          string
		path          string
		expectedValue any
		expectFound   bool
	}{
		{
			name:          "Simple top level key",
			path:          "top_level",
			expectedValue: "value",
			expectFound:   true,
		},
		{
			name:          "Two levels deep",
			path:          "database.host",
			expectedValue: "localhost",
			expectFound:   true,
		},
		{
			name:          "Three levels deep",
			path:          "database.connection.pool_size",
			expectedValue: 10,
			expectFound:   true,
		},
		{
			name:          "Top level key not found",
			path:          "not_found",
			expectedValue: nil,
			expectFound:   false,
		},
		{
			name:          "Nested key not found",
			path:          "database.user",
			expectedValue: nil,
			expectFound:   false,
		},
		{
			name:          "Path through a non-map value",
			path:          "a.b.c",
			expectedValue: nil,
			expectFound:   false,
		},
		{
			name:          "Empty path",
			path:          "",
			expectedValue: nil,
			expectFound:   false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			val, found := LookupNestedValue(data, tc.path)
			assert.Equal(t, tc.expectFound, found)
			if tc.expectFound {
				assert.Equal(t, tc.expectedValue, val)
			}
		})
	}
}

func TestSetNestedValue(t *testing.T) {
	t.Run("Setting new nested value should create maps", func(t *testing.T) {
		dest := make(Values)
		err := SetNestedValue(dest, "a.b.c", "hello")
		require.NoError(t, err)

		expected := Values{
			"a": Values{
				"b": Values{
					"c": "hello",
				},
			},
		}
		assert.Equal(t, expected, dest)
	})

	t.Run("Setting value should overwrite existing value", func(t *testing.T) {
		dest := Values{
			"a": Values{
				"b": "old_value",
			},
		}
		err := SetNestedValue(dest, "a.b", "new_value")
		require.NoError(t, err)
		assert.Equal(t, "new_value", dest["a"].(Values)["b"])
	})

	t.Run("Should return error when path segment is not a map", func(t *testing.T) {
		dest := Values{
			"a": Values{
				"b": "i am not a map",
			},
		}
		err := SetNestedValue(dest, "a.b.c", "value")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "segment \"a.b\" is not a map")
	})
}

func TestDeepMerge(t *testing.T) {
	t.Run("Simple merge should combine keys", func(t *testing.T) {
		map1 := Values{"a": 1}
		map2 := Values{"b": 2}
		merged := DeepMerge(map1, map2)
		expected := Values{"a": 1, "b": 2}
		assert.Equal(t, expected, merged)
	})

	t.Run("Later maps should overwrite earlier maps", func(t *testing.T) {
		map1 := Values{"a": 1, "b": "old"}
		map2 := Values{"b": "new", "c": 3}
		merged := DeepMerge(map1, map2)
		expected := Values{"a": 1, "b": "new", "c": 3}
		assert.Equal(t, expected, merged)
	})

	t.Run("Nested maps should be merged recursively", func(t *testing.T) {
		map1 := Values{
			"server": Values{
				"host": "localhost",
				"port": 8080,
			},
		}
		map2 := Values{
			"server": Values{
				"port": 9090,    // Overwrite
				"user": "admin", // Add
			},
		}
		merged := DeepMerge(map1, map2)
		expected := Values{
			"server": Values{
				"host": "localhost",
				"port": 9090,
				"user": "admin",
			},
		}
		assert.Equal(t, expected, merged)
	})

	t.Run("Three-way merge should work correctly", func(t *testing.T) {
		globals := Values{"env": "dev", "timeout": 10}
		target := Values{"env": "prod", "retries": 3}
		template := Values{"timeout": 30, "feature_flag": true}

		merged := DeepMerge(globals, target, template)
		expected := Values{
			"env":          "prod",
			"timeout":      30,
			"retries":      3,
			"feature_flag": true,
		}
		assert.Equal(t, expected, merged)
	})

	t.Run("Merging should not modify original maps", func(t *testing.T) {
		original := Values{
			"a": Values{
				"b": 1,
			},
		}
		overwrite := Values{
			"a": Values{
				"c": 2,
			},
		}
		_ = DeepMerge(original, overwrite)
		// Check that the original map was not mutated
		require.Equal(t, 1, len(original["a"].(Values)))
		assert.Equal(t, 1, original["a"].(Values)["b"])
	})
}
