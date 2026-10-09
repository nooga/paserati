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

// SetProperty sets a property's type, remembering the order of first insertion.
func (ot *ObjectType) SetProperty(name string, t Type) {
	if ot.Properties == nil {
		ot.Properties = make(map[string]Type)
	}
	if _, exists := ot.Properties[name]; !exists {
		ot.PropertyOrder = append(ot.PropertyOrder, name)
	}
	ot.Properties[name] = t
}

// PropertyNames returns the type's own property names in declaration order.
// Names added without SetProperty come last, sorted.
func (ot *ObjectType) PropertyNames() []string {
	names := make([]string, 0, len(ot.Properties))
	seen := make(map[string]bool, len(ot.Properties))
	for _, n := range ot.PropertyOrder {
		if _, ok := ot.Properties[n]; ok && !seen[n] {
			seen[n] = true
			names = append(names, n)
		}
	}
	if len(names) < len(ot.Properties) {
		var rest []string
		for n := range ot.Properties {
			if !seen[n] {
				rest = append(rest, n)
			}
		}
		sort.Strings(rest)
		names = append(names, rest...)
	}
	return names
}

// EffectivePropertyNames returns own and inherited property names in
// declaration order: members inherited through extends come first, in
// extends order, then the type's own members. A redeclared member keeps the
// position of its first declaration.
func (ot *ObjectType) EffectivePropertyNames() []string {
	var names []string
	seen := make(map[string]bool)
	add := func(n string) {
		if !seen[n] {
			seen[n] = true
			names = append(names, n)
		}
	}
	for _, bt := range ot.BaseTypes {
		if baseObj, ok := resolveBaseType(bt).(*ObjectType); ok && baseObj != ot {
			for _, n := range baseObj.EffectivePropertyNames() {
				add(n)
			}
		}
	}
	for _, n := range ot.PropertyNames() {
		add(n)
	}
	return names
}

// SetOrdered writes m[name] and records first-insertion order in *order, for
// code that builds a property map before the ObjectType exists.
func SetOrdered(m map[string]Type, order *[]string, name string, t Type) {
	if _, exists := m[name]; !exists {
		*order = append(*order, name)
	}
	m[name] = t
}

// SetPropertyDoc records the JSDoc comment of an own property. An empty doc
// is ignored.
func (ot *ObjectType) SetPropertyDoc(name, doc string) {
	if doc == "" {
		return
	}
	if ot.PropertyDocs == nil {
		ot.PropertyDocs = make(map[string]string)
	}
	ot.PropertyDocs[name] = doc
}

// PropertyDoc returns the JSDoc comment of a property, looking through
// extends clauses when the type does not document the member itself.
func (ot *ObjectType) PropertyDoc(name string) string {
	if doc, ok := ot.PropertyDocs[name]; ok {
		return doc
	}
	if _, own := ot.Properties[name]; own {
		return ""
	}
	for _, bt := range ot.BaseTypes {
		if baseObj, ok := resolveBaseType(bt).(*ObjectType); ok && baseObj != ot {
			if doc := baseObj.PropertyDoc(name); doc != "" {
				return doc
			}
		}
	}
	return ""
}

// CopyDisplay gives clone the name this type prints with, mapping the type
// arguments of a generic class instance through sub.
func (ot *ObjectType) CopyDisplay(clone *ObjectType, sub func(Type) Type) {
	clone.DisplayName = ot.DisplayName
	if ot.GenericName == "" {
		return
	}
	clone.GenericName = ot.GenericName
	clone.GenericArgs = make([]Type, len(ot.GenericArgs))
	for i, a := range ot.GenericArgs {
		clone.GenericArgs[i] = sub(a)
	}
}
