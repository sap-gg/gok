package render

import (
	"context"
	"fmt"
	"io"
	"os"
	"reflect"
	"strings"

	"github.com/rs/zerolog/log"

	"github.com/sap-gg/gok/internal"
)

type (
	Values       map[string]any
	ScopedValues map[string]Values
)

// ValuesOverwritesSpec are global or per-target overwrites
type ValuesOverwritesSpec struct {
	Version int

	// Values are global value overwrites
	Values Values

	// Target are overwrites per taget
	Targets map[string]*ValuesTargetOverwrites
}

func NewValuesOverwritesSpec() *ValuesOverwritesSpec {
	return &ValuesOverwritesSpec{
		Version: internal.OverwritesFileVersion,
		Values:  make(Values),
		Targets: make(map[string]*ValuesTargetOverwrites),
	}
}

// ValuesTargetOverwrites are overwrites specific to a target.
type ValuesTargetOverwrites struct {
	Values Values
}

func NewValuesTargetOverwrites() *ValuesTargetOverwrites {
	return &ValuesTargetOverwrites{
		Values: make(Values),
	}
}

func parseValuesOverwrites(ctx context.Context, path string) (*ValuesOverwritesSpec, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("opening file %q: %w", path, err)
	}
	defer f.Close()

	var spec ValuesOverwritesSpec
	if err := internal.NewYAMLDecoder(f).DecodeContext(ctx, &spec); err != nil {
		if internal.IsDecodeErrorAndPrint(err) {
			return nil, fmt.Errorf("parsing spec")
		}
		return nil, fmt.Errorf("decoding overwrites %q: %w", path, err)
	}

	if spec.Version != internal.OverwritesFileVersion {
		return nil, fmt.Errorf("unsupported overwrites version %d (expected %d)",
			spec.Version, internal.OverwritesFileVersion)
	}

	return &spec, nil
}

// ParseValuesOverwrites parses multiple overwrite files and merges them into one single spec.
func ParseValuesOverwrites(ctx context.Context, paths []string) (*ValuesOverwritesSpec, error) {
	result := NewValuesOverwritesSpec()
	for _, p := range paths {
		o, err := parseValuesOverwrites(ctx, p)
		if err != nil {
			return nil, err
		}
		result.Values = DeepMerge(result.Values, o.Values)
		for target, targetValues := range o.Targets {
			if _, ok := result.Targets[target]; !ok {
				result.Targets[target] = NewValuesTargetOverwrites()
			}
			result.Targets[target].Values = DeepMerge(result.Targets[target].Values, targetValues.Values)
		}
	}
	return result, nil
}

func ParseStringToStringValuesOverwrites(_ context.Context, m map[string]string) (*ValuesOverwritesSpec, error) {
	result := NewValuesOverwritesSpec()

	// [<target>].value=v, or:
	// value=v, or:
	// nested.value=v
	for k, v := range m {
		// target-specific value
		if strings.HasPrefix(k, "@") && strings.Contains(k, ".") {
			dot := strings.Index(k, ".")

			targetID := k[1:dot]
			if _, ok := result.Targets[targetID]; !ok {
				result.Targets[targetID] = NewValuesTargetOverwrites()
			}

			k = k[dot+1:]
			if err := SetNestedValue(result.Targets[targetID].Values, k, v); err != nil {
				return nil, fmt.Errorf("setting target value %q: %w", k, err)
			}
		} else {
			if err := SetNestedValue(result.Values, k, v); err != nil {
				return nil, fmt.Errorf("setting global value %q: %w", k, err)
			}
		}
	}

	return result, nil
}

func (s *ValuesOverwritesSpec) ValuesForTarget(targetID string) Values {
	var v Values
	// first apply all global external values
	if s.Values != nil {
		v = s.Values
	} else {
		v = make(Values)
	}
	// then any target specific values
	if vals, ok := s.Targets[targetID]; ok {
		v = DeepMerge(v, vals.Values)
	}
	return v
}

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
		if internal.IsDecodeErrorAndPrint(err) {
			return nil, fmt.Errorf("parsing values")
		}
		return nil, fmt.Errorf("decode values file %q: %w", path, err)
	}

	return values, nil
}

// CollectStrings recursively walks a map or slice and collects all string leaf values.
func CollectStrings(data any) []string {
	var res []string
	val := reflect.ValueOf(data)

	switch val.Kind() {
	case reflect.Map:
		for _, key := range val.MapKeys() {
			res = append(res, CollectStrings(val.MapIndex(key).Interface())...)
		}
	case reflect.Slice:
		for i := 0; i < val.Len(); i++ {
			res = append(res, CollectStrings(val.Index(i).Interface())...)
		}
	case reflect.String:
		if s := val.String(); s != "" {
			res = append(res, s)
		}
	default:
		// ignore other types
		log.Trace().Msgf("ignoring non-string leaf of type %s", val.Kind())
	}

	return res
}

// LookupNestedValue traverses a map using a dot-separated path and returns the value if found.
func LookupNestedValue(data map[string]any, path string) (any, bool) {
	keys := strings.Split(path, ".")
	current := any(data)

	for _, key := range keys {
		val := reflect.ValueOf(current)
		if val.Kind() != reflect.Map {
			return nil, false // cannot traverse non-map
		}
		// check if key exists
		keyValue := val.MapIndex(reflect.ValueOf(key))
		if !keyValue.IsValid() {
			return nil, false // key not found
		}
		current = keyValue.Interface()
	}

	return current, true
}

// SetNestedValue populates a map using a dot-separated path string, creating nested maps as needed.
func SetNestedValue(dest Values, path string, value any) error {
	keys := strings.Split(path, ".")
	current := dest

	// traverse / create all but the last key
	for i, key := range keys[:len(keys)-1] {
		if _, ok := current[key]; !ok {
			current[key] = make(Values)
		}
		if next, ok := current[key].(Values); ok {
			current = next
		} else {
			// This happens if a path segment is already a non-map value.
			// e.g., trying to set "a.b.c" when "a.b" is already "hello".
			return fmt.Errorf("cannot set nested value at %q: segment %q is not a map",
				path, strings.Join(keys[:i+1], "."))
		}
	}

	// set the final key
	finalKey := keys[len(keys)-1]
	current[finalKey] = value
	return nil
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
