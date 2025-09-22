package render

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Helper function to create a mock manifest for our tests.
func createMockManifest() *Manifest {
	return &Manifest{
		Targets: map[string]*ManifestTarget{
			"proxy": {
				ID:   "proxy",
				Tags: []string{"production", "networking"},
			},
			"survival": {
				ID:   "survival",
				Tags: []string{"production", "gameplay"},
			},
			"creative": {
				ID:   "creative",
				Tags: []string{"gameplay"},
			},
			"dev-proxy": {
				ID:   "dev-proxy",
				Tags: []string{"development", "networking"},
			},
		},
	}
}

func TestSelectTargets(t *testing.T) {
	manifest := createMockManifest()

	testCases := []struct {
		name        string
		allFlag     bool
		names       []string
		tags        []string
		expectedIDs []string
		expectError bool
	}{
		{
			name:        "Select a single target by name",
			names:       []string{"proxy"},
			expectedIDs: []string{"proxy"},
		},
		{
			name:        "Select multiple targets by name",
			names:       []string{"survival", "creative"},
			expectedIDs: []string{"survival", "creative"},
		},
		{
			name:        "Select all targets with 'production' tag",
			tags:        []string{"production"},
			expectedIDs: []string{"proxy", "survival"},
		},
		{
			name:        "Select all targets with 'networking' tag",
			tags:        []string{"networking"},
			expectedIDs: []string{"proxy", "dev-proxy"},
		},
		{
			name:        "Select with multiple tags",
			tags:        []string{"production", "development"},
			expectedIDs: []string{"proxy", "survival", "dev-proxy"},
		},
		{
			name:        "Select with both name and tag (no duplicates)",
			names:       []string{"proxy"},
			tags:        []string{"gameplay"},
			expectedIDs: []string{"proxy", "survival", "creative"},
		},
		{
			name:    "Select all targets with the 'all' flag",
			allFlag: true,
			expectedIDs: []string{
				"proxy",
				"survival",
				"creative",
				"dev-proxy",
			}, // Order may vary, so we'll check presence
		},
		{
			name:        "Select a target that does not exist",
			names:       []string{"non-existent"},
			expectError: true,
		},
		{
			name:        "Select with a tag that does not exist",
			tags:        []string{"non-existent-tag"},
			expectedIDs: []string{}, // Should return empty, not error
		},
		{
			name:        "Select with no flags",
			expectedIDs: []string{},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			selected, err := SelectTargets(manifest, tc.allFlag, tc.names, tc.tags)

			if tc.expectError {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)

			selectedIDs := make([]string, len(selected))
			for i, target := range selected {
				selectedIDs[i] = target.ID
			}

			// For the 'all' case, order is not guaranteed by map iteration.
			// For all other cases, the implementation preserves order, so we can assert equality.
			if tc.allFlag {
				assert.ElementsMatch(t, tc.expectedIDs, selectedIDs)
			} else {
				assert.Equal(t, tc.expectedIDs, selectedIDs)
			}
		})
	}
}
