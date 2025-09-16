package render

type (
	Values       map[string]any
	ScopedValues map[string]Values
)
type TemplateData struct {
	Values ScopedValues `json:"values"`
}

func NewTemplateData(
	allValues ScopedValues,
	spec *GlobalSpec,
	target *ManifestTarget,
	template *TemplateSpec,
) *TemplateData {
	scopedValues := make(ScopedValues, len(allValues)+1)
	for k, v := range allValues {
		scopedValues[k] = v
	}

	var globalVars Values
	if spec != nil {
		globalVars = spec.Values
	}

	scopedValues["current"] = DeepMerge(globalVars, target.Values, template.Values)

	return &TemplateData{
		Values: scopedValues,
	}
}

// PreprocessValues prepares the values for all targets by merging global values into each target's values.
// It does not contain template-specific values.
func PreprocessValues(m *Manifest) ScopedValues {
	allValues := make(ScopedValues, len(m.Targets)+1)

	// add global values under the "global" key, e.g.
	// {{ .values.global.someKey }}
	var globalVars Values
	if m.Globals != nil {
		globalVars = m.Globals.Values
	}
	allValues["global"] = globalVars

	for id, t := range m.Targets {
		allValues[id] = t.Values
	}

	return allValues
}

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
