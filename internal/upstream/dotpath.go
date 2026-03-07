package upstream

import "strings"

// resolveByDotPath traverses a nested map using a dot-separated key path.
// For example, "meta.results.total" traverses parsed["meta"]["results"]["total"].
// Returns the value and true if found, or nil and false if any key is missing.
func resolveByDotPath(parsed map[string]interface{}, dotPath string) (interface{}, bool) {
	keys := strings.Split(dotPath, ".")
	var current interface{} = parsed

	for _, key := range keys {
		m, ok := current.(map[string]interface{})
		if !ok {
			return nil, false
		}
		current, ok = m[key]
		if !ok {
			return nil, false
		}
	}

	return current, true
}
