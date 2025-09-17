package render

type (
	Values       map[string]any
	ScopedValues map[string]Values
)

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
