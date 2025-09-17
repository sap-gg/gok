package merge

// DeepMergeMaps performs a deep merge of multiple maps.
// Keys in later maps recursively overwrite keys in earlier ones.
func DeepMergeMaps(maps ...map[string]any) map[string]any {
	result := make(map[string]any)
	for _, m := range maps {
		for k, v := range m {
			if v, ok := v.(map[string]any); ok {
				if dest, ok := result[k].(map[string]any); ok {
					// If the key exists in the destination and both are maps, merge them recursively.
					result[k] = DeepMergeMaps(dest, v)
					continue
				}
			}
			// Otherwise, just set the value.
			result[k] = v
		}
	}
	return result
}
