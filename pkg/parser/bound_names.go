package parser

// Binding-name enumeration for declarations.
//
// Several passes need "what names does this declaration introduce?": var
// hoisting, let/const TDZ pre-registration, and module export registration.
// A destructuring pattern makes that a recursive question, and the recursion
// has two shapes to cover, because a pattern is represented differently at its
// top level than when nested:
//
//   - At the top level of a declaration, a default lives in
//     DestructuringProperty.Default / DestructuringElement.Default and a rest
//     element in DestructuringElement.IsRest, with Target holding the binding.
//   - Nested one level down, Target is a raw ObjectLiteral/ArrayLiteral
//     expression, so a default appears as an AssignmentExpression and a rest as
//     a SpreadElement, and the binding is inside those wrappers.
//
// Missing the wrapped forms is what made a name behind a nested default or rest
// invisible to var hoisting: `function f(){ { var {a: {b = 1}} = o; } return b; }`
// threw ReferenceError even though the same declaration outside a block bound b
// correctly.

// CollectPatternNames appends the binding names in a destructuring pattern
// target to *names, in source order, skipping any name already in seen.
func CollectPatternNames(target Expression, seen map[string]bool, names *[]string) {
	if target == nil {
		return
	}
	switch t := target.(type) {
	case *Identifier:
		if !seen[t.Value] {
			*names = append(*names, t.Value)
			seen[t.Value] = true
		}
	case *AssignmentExpression:
		// A nested default: `{a: {b = 1}}` / `[[m = 4]]`. The binding is the
		// assignment's left-hand side.
		CollectPatternNames(t.Left, seen, names)
	case *SpreadElement:
		// A nested rest element: `[[x, ...ys]]`.
		CollectPatternNames(t.Argument, seen, names)
	case *ObjectLiteral:
		// Nested object pattern: `{a: {b, c}}`
		for _, prop := range t.Properties {
			if prop == nil {
				continue
			}
			if prop.Value != nil {
				// Key: Value pattern - the target is in Value.
				CollectPatternNames(prop.Value, seen, names)
			} else if prop.Key != nil {
				// Shorthand `{a}`, or a rest `{...r}`, which the parser stores as
				// a SpreadElement in Key. Either way the Key is the target.
				CollectPatternNames(prop.Key, seen, names)
			}
		}
	case *ArrayLiteral:
		// Nested array pattern: `[a, [b, c]]`
		for _, elem := range t.Elements {
			CollectPatternNames(elem, seen, names)
		}
	}
}

// CollectObjectPatternNames appends the binding names of an object pattern's
// properties and rest property to *names.
func CollectObjectPatternNames(props []*DestructuringProperty, rest *DestructuringElement, seen map[string]bool, names *[]string) {
	for _, prop := range props {
		if prop == nil {
			continue
		}
		CollectPatternNames(prop.Target, seen, names)
	}
	if rest != nil {
		CollectPatternNames(rest.Target, seen, names)
	}
}

// CollectArrayPatternNames appends the binding names of an array pattern's
// elements to *names.
func CollectArrayPatternNames(elements []*DestructuringElement, seen map[string]bool, names *[]string) {
	for _, elem := range elements {
		if elem == nil {
			continue
		}
		CollectPatternNames(elem.Target, seen, names)
	}
}

// CollectDeclaredNames appends the binding names a single declaration statement
// introduces to *names, in source order. It covers a let/const/var declarator
// list (every declarator, not just the first) and a destructuring declaration;
// anything else contributes nothing.
//
// This looks at one statement only - it does not recurse into nested statement
// lists, and it does not filter by declaration keyword. Callers that need
// either (var hoisting walks bodies and wants only `var`; TDZ registration
// wants only let/const) do that themselves.
func CollectDeclaredNames(stmt Statement, seen map[string]bool, names *[]string) {
	switch s := stmt.(type) {
	case *LetStatement:
		collectDeclaratorNames(s.Declarations, s.Name, seen, names)
	case *ConstStatement:
		collectDeclaratorNames(s.Declarations, s.Name, seen, names)
	case *VarStatement:
		collectDeclaratorNames(s.Declarations, s.Name, seen, names)
	case *ObjectDestructuringDeclaration:
		CollectObjectPatternNames(s.Properties, s.RestProperty, seen, names)
	case *ArrayDestructuringDeclaration:
		CollectArrayPatternNames(s.Elements, seen, names)
	case *DeclarationGroup:
		for _, d := range s.Declarations {
			CollectDeclaredNames(d, seen, names)
		}
	}
}

// DeclaredNames is CollectDeclaredNames with its own seen map, for a one-off
// query about a single statement.
func DeclaredNames(stmt Statement) []string {
	var names []string
	seen := make(map[string]bool)
	CollectDeclaredNames(stmt, seen, &names)
	return names
}

// collectDeclaratorNames reads the declarator list, falling back to the
// legacy first-declarator Name field for the parser paths that only set that.
func collectDeclaratorNames(declarators []*VarDeclarator, legacyName *Identifier, seen map[string]bool, names *[]string) {
	if len(declarators) == 0 {
		if legacyName != nil && !seen[legacyName.Value] {
			*names = append(*names, legacyName.Value)
			seen[legacyName.Value] = true
		}
		return
	}
	for _, d := range declarators {
		if d == nil || d.Name == nil {
			continue
		}
		if !seen[d.Name.Value] {
			*names = append(*names, d.Name.Value)
			seen[d.Name.Value] = true
		}
	}
}
