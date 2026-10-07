package types

import "sort"

// SortedPropertyNames returns the keys of a property map in a deterministic
// order. Go randomizes map iteration, so anything whose result depends on the
// order properties are visited (type printing, inference, union construction)
// must go through this rather than ranging over the map directly.
func SortedPropertyNames(props map[string]Type) []string {
	names := make([]string, 0, len(props))
	for name := range props {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
