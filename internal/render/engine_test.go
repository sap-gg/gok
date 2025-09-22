package render

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sap-gg/gok/internal/strategy"
	"github.com/sap-gg/gok/internal/templ"
)

func TestEngineValuePrecedence(t *testing.T) {
	tempDir := t.TempDir()

	manifestContent := `
version: 1
values:
  my_value: "1. from manifest global"
targets:
  my-target:
    output: "output"
    values:
      my_value: "2. from target"
    templates:
      - from: ./template
        values:
          my_value: "3. from template spec"
`
	manifestPath := filepath.Join(tempDir, "gok-manifest.yaml")
	require.NoError(t, os.WriteFile(manifestPath, []byte(manifestContent), 0644))

	templateDir := filepath.Join(tempDir, "template")
	require.NoError(t, os.Mkdir(templateDir, 0755))

	templateFileContent := `result: "{{ .values.my_value }}"`
	templateFilePath := filepath.Join(templateDir, "result.yaml.templ")
	require.NoError(t, os.WriteFile(templateFilePath, []byte(templateFileContent), 0644))

	templateManifestContent := `
version: 1
imports:
  values:
    "my_value":
      description: "A test value"
`
	templateManifestPath := filepath.Join(templateDir, "gok-template.yaml")
	require.NoError(t, os.WriteFile(templateManifestPath, []byte(templateManifestContent), 0644))

	// --- External Values File (-f flag) ---
	externalValuesContent := `
version: 1

values:
  my_value: "4. from external file"
`
	externalValuesPath := filepath.Join(tempDir, "external-values.yaml")
	require.NoError(t, os.WriteFile(externalValuesPath, []byte(externalValuesContent), 0644))

	// --- CLI Overwrite Values (-v flag) ---
	cliOverwritesMap := map[string]string{
		"my_value": "5. from CLI overwrite",
	}

	// 2. Execution: Run the render engine
	ctx := context.Background()

	// Load all the value sources
	manifest, manifestDir, err := ReadManifest(ctx, manifestPath)
	require.NoError(t, err)

	externalValues, err := ParseValuesOverwrites(ctx, []string{externalValuesPath})
	require.NoError(t, err)

	cliOverwrites, err := ParseStringToStringValuesOverwrites(ctx, cliOverwritesMap)
	require.NoError(t, err)

	// Pre-compute the values for cross-target lookups
	resolvedTargetValues, err := PreComputeAllTargetValues(manifest, externalValues, cliOverwrites)
	require.NoError(t, err)

	// Create the engine
	workDir := t.TempDir()
	renderer := templ.NewTemplateRenderer()
	registry, err := strategy.NewRegistry(&strategy.CopyOnlyStrategy{Overwrite: true}, nil)
	require.NoError(t, err)

	engine, err := NewEngine(
		manifestDir,
		workDir,
		renderer,
		registry,
		manifest.Values,
		nil, // No secrets for this test
		externalValues,
		cliOverwrites,
		resolvedTargetValues,
	)
	require.NoError(t, err)

	target, ok := manifest.Targets["my-target"]
	require.True(t, ok)
	err = engine.RenderTarget(ctx, target)
	require.NoError(t, err)

	outputFilePath := filepath.Join(workDir, "output", "result.yaml")
	outputBytes, err := os.ReadFile(outputFilePath)
	require.NoError(t, err)

	// The output should contain the value with the highest precedence
	expectedOutput := "result: \"5. from CLI overwrite\""
	assert.Contains(t, string(outputBytes), expectedOutput)

	t.Logf("Final rendered output:\n%s", string(outputBytes))
}
