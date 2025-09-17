package render

import (
	"context"
	"fmt"
	"io"
	"os"
	"reflect"

	"github.com/rs/zerolog/log"

	"github.com/sap-gg/gok/internal"
)

type (
	Values       map[string]any
	ScopedValues map[string]Values
)

// LoadValuesFiles reads a list of YAML file paths, parses them, and merges them.
// It supports reading from stdin by using "-" as a path.
func LoadValuesFiles(ctx context.Context, paths []string) (Values, error) {
	mergedValues := make(Values)

	for _, path := range paths {
		values, err := loadValuesFile(ctx, path)
		if err != nil {
			return nil, err
		}
		mergedValues = DeepMerge(mergedValues, values)
	}

	return mergedValues, nil
}

func loadValuesFile(ctx context.Context, path string) (Values, error) {
	var content io.Reader

	// allow reading from stdin if path is "-"
	if path == "-" {
		content = os.Stdin
	} else {
		f, err := os.Open(path)
		if err != nil {
			return nil, fmt.Errorf("open values file %q: %w", path, err)
		}
		defer f.Close()
		content = f
	}

	var values Values
	if err := internal.NewYAMLDecoder(content).DecodeContext(ctx, &values); err != nil {
		return nil, fmt.Errorf("decode values file %q: %w", path, err)
	}

	return values, nil
}

// CollectStrings recursively walks a map or slice and collects all string leaf values.
func CollectStrings(data any) []string {
	var strings []string
	val := reflect.ValueOf(data)

	switch val.Kind() {
	case reflect.Map:
		for _, key := range val.MapKeys() {
			strings = append(strings, CollectStrings(val.MapIndex(key).Interface())...)
		}
	case reflect.Slice:
		for i := 0; i < val.Len(); i++ {
			strings = append(strings, CollectStrings(val.Index(i).Interface())...)
		}
	case reflect.String:
		if s := val.String(); s != "" {
			strings = append(strings, s)
		}
	default:
		// ignore other types
		log.Trace().Msgf("ignoring non-string leaf of type %s", val.Kind())
	}

	return strings
}

// DeepMerge merges multiple Values maps into one, from left to right.
// Nested maps are merged recursively, while scalar values are overwritten by later maps.
func DeepMerge(maps ...Values) Values {
	out := make(Values)
	for _, m := range maps {
		mergeInto(out, m)
	}
	return out
}

func mergeInto(dst, src Values) {
	if src == nil {
		return
	}
	for k, v := range src {
		if sv, ok := v.(map[string]any); ok {
			if dv, ok := dst[k]; ok {
				if dm, ok := dv.(map[string]any); ok {
					mergeInto(dm, sv)
					continue
				}
			}
			// copy nested map
			cpy := make(map[string]any, len(sv))
			mergeInto(cpy, sv)
			dst[k] = cpy
			continue
		}
		// scalar or non-map, overwrite
		dst[k] = v
	}
}
