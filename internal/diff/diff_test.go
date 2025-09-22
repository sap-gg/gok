package diff

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sap-gg/gok/internal"
	"github.com/sap-gg/gok/internal/lockfile"
)

func setupDiffDirs(t *testing.T, oldState, newState, actualState map[string]string) (currentDir, desiredDir string) {
	currentDir = t.TempDir()
	desiredDir = t.TempDir()

	oldLock := &lockfile.LockFile{Version: 1, Files: make(lockfile.LockFiles)}
	for path, content := range oldState {
		p := filepath.Join(currentDir, path)
		require.NoError(t, os.MkdirAll(filepath.Dir(p), 0755))
		require.NoError(t, os.WriteFile(p, []byte(content), 0644))
		hash, _ := lockfile.FileSHA256(p)
		oldLock.Files[path] = &lockfile.LockEntry{Hash: hash}
	}
	for path, content := range actualState {
		p := filepath.Join(currentDir, path)
		require.NoError(t, os.MkdirAll(filepath.Dir(p), 0755))
		require.NoError(t, os.WriteFile(p, []byte(content), 0644))
	}
	lockPath := filepath.Join(currentDir, internal.LockFileName)
	f, _ := os.Create(lockPath)
	_ = internal.NewYAMLEncoder(f).Encode(oldLock)
	f.Close()

	for path, content := range newState {
		p := filepath.Join(desiredDir, path)
		require.NoError(t, os.MkdirAll(filepath.Dir(p), 0755))
		require.NoError(t, os.WriteFile(p, []byte(content), 0644))
	}
	require.NoError(t, lockfile.Create(context.Background(), desiredDir))

	return currentDir, desiredDir
}

func TestComparer_Compare(t *testing.T) {
	testCases := []struct {
		name         string
		oldState     map[string]string // State according to old lock file
		newState     map[string]string // State according to new lock file
		actualState  map[string]string // State currently on disk
		expectedPath string
		expectedType Type
	}{
		{
			name:         "should detect created file",
			oldState:     map[string]string{},
			newState:     map[string]string{"new.txt": "content"},
			actualState:  map[string]string{},
			expectedPath: "new.txt",
			expectedType: Created,
		},
		{
			name:         "should detect removed file",
			oldState:     map[string]string{"removed.txt": "content"},
			newState:     map[string]string{},
			actualState:  map[string]string{}, // File is already physically removed
			expectedPath: "removed.txt",
			expectedType: Removed,
		},
		{
			name:         "should detect modified file",
			oldState:     map[string]string{"modified.txt": "old"},
			newState:     map[string]string{"modified.txt": "new"},
			actualState:  map[string]string{"modified.txt": "old"}, // On disk state matches old lock
			expectedPath: "modified.txt",
			expectedType: Modified,
		},
		{
			name:         "should detect conflict on modified file",
			oldState:     map[string]string{"conflict.txt": "old"},
			newState:     map[string]string{"conflict.txt": "new"},
			actualState:  map[string]string{"conflict.txt": "MANUALLY EDITED"}, // On disk state has drifted
			expectedPath: "conflict.txt",
			expectedType: Conflict,
		},
		{
			name:         "should detect conflict on removed file",
			oldState:     map[string]string{"removed.txt": "old"},
			newState:     map[string]string{},
			actualState:  map[string]string{"removed.txt": "MANUALLY EDITED"}, // File should be gone, but was edited
			expectedPath: "removed.txt",
			expectedType: Conflict,
		},
		{
			name:         "should report nothing for unchanged file",
			oldState:     map[string]string{"unchanged.txt": "content"},
			newState:     map[string]string{"unchanged.txt": "content"},
			actualState:  map[string]string{"unchanged.txt": "content"},
			expectedPath: "unchanged.txt",
			expectedType: Unchanged,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			currentDir, desiredDir := setupDiffDirs(t, tc.oldState, tc.newState, tc.actualState)
			comparer := NewComparer(currentDir, desiredDir)
			report, err := comparer.Compare()
			require.NoError(t, err)

			if tc.expectedType == Unchanged {
				assert.NotContains(t, report.Changes, tc.expectedPath)
				assert.False(t, report.HasChanges())
			} else {
				require.Contains(t, report.Changes, tc.expectedPath)
				assert.Equal(t, tc.expectedType, report.Changes[tc.expectedPath].Type)
				assert.True(t, report.HasChanges())
				if tc.expectedType == Conflict {
					assert.True(t, report.HasConflicts())
				}
			}
		})
	}
}
