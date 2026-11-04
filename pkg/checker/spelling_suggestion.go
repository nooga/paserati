package checker

import (
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/nooga/paserati/pkg/errors"
	"github.com/nooga/paserati/pkg/parser"
	"github.com/nooga/paserati/pkg/types"
)

// getSpellingSuggestion ports TypeScript's core.ts `getSpellingSuggestion`
// algorithm. Given an unresolved name and the set of candidate names visible
// in scope, it returns the closest candidate that is "close enough" to
// plausibly be a typo for name, or "" when nothing is close enough to be
// worth suggesting.
//
// The heuristics (length-difference cutoff, minimum-length case-insensitive
// exemption, and the distance threshold) mirror TypeScript's implementation
// so our diagnostics agree with tsc on when to emit a "Did you mean" hint
// (TS2552) instead of a plain "Cannot find name" (TS2304).
func getSpellingSuggestion(name string, candidates []string) string {
	if name == "" {
		return ""
	}

	nameRunes := []rune(name)
	nameLen := len(nameRunes)

	maximumLengthDifference := maxInt(2, int(float64(nameLen)*0.34))
	// Candidates worse (larger distance) than this are not worth suggesting.
	// Starts as the "no suggestion at all" threshold and tightens as better
	// candidates are found, so later candidates must beat earlier ones.
	bestDistance := float64(int(float64(nameLen)*0.4)) + 1
	bestCandidate := ""

	for _, candidate := range candidates {
		if candidate == "" || candidate == name {
			continue
		}
		candidateRunes := []rune(candidate)
		candidateLen := len(candidateRunes)

		lengthDifference := candidateLen - nameLen
		if lengthDifference < 0 {
			lengthDifference = -lengthDifference
		}
		if lengthDifference > maximumLengthDifference {
			continue
		}
		// Only consider candidates shorter than 3 characters when they differ
		// from the target by case alone (matches TypeScript: short names like
		// "of"/"if" are too easy to collide with by chance).
		if candidateLen < 3 && !strings.EqualFold(candidate, name) {
			continue
		}

		distance, ok := levenshteinWithMax(nameRunes, candidateRunes, bestDistance-0.1)
		if !ok {
			continue
		}
		bestDistance = distance
		bestCandidate = candidate
	}

	return bestCandidate
}

// levenshteinWithMax computes a weighted Levenshtein edit distance between s1
// and s2, ported from TypeScript's levenshteinWithMax. Substitutions cost 2,
// unless the two runes differ only in case, in which case they cost 0.1;
// insertions and deletions cost 1. If the best achievable cost for a row ever
// exceeds max, the function bails out early and reports ok=false to signal
// "too far to matter" without paying for the rest of the computation.
func levenshteinWithMax(s1, s2 []rune, max float64) (distance float64, ok bool) {
	previous := make([]float64, len(s2)+1)
	current := make([]float64, len(s2)+1)

	for j := 0; j <= len(s2); j++ {
		previous[j] = float64(j)
	}

	for i := 1; i <= len(s1); i++ {
		c1 := s1[i-1]
		current[0] = float64(i)

		minJ := 1
		if float64(i) > max {
			minJ = int(math.Ceil(float64(i) - max))
		}
		maxJ := len(s2)
		if v := i + int(max); v < maxJ {
			maxJ = v
		}

		// Columns to the left of minJ are unreachable within budget; fill them
		// with a value guaranteed to exceed max so they can't be selected as a
		// minimum below.
		for j := 1; j < minJ; j++ {
			current[j] = max + 1
		}

		rowMin := float64(i)
		for j := minJ; j <= maxJ; j++ {
			var substitutionDistance float64
			c2 := s2[j-1]
			if c1 == c2 {
				substitutionDistance = previous[j-1]
			} else if runeEqualFold(c1, c2) {
				substitutionDistance = previous[j-1] + 0.1
			} else {
				substitutionDistance = previous[j-1] + 2
			}

			deletionDistance := previous[j] + 1
			insertionDistance := current[j-1] + 1

			best := substitutionDistance
			if deletionDistance < best {
				best = deletionDistance
			}
			if insertionDistance < best {
				best = insertionDistance
			}
			current[j] = best

			if best < rowMin {
				rowMin = best
			}
		}
		for j := maxJ + 1; j <= len(s2); j++ {
			current[j] = max + 1
		}

		if rowMin > max {
			return 0, false
		}

		previous, current = current, previous
	}

	result := previous[len(s2)]
	if result > max {
		return 0, false
	}
	return result, true
}

func runeEqualFold(a, b rune) bool {
	if a == b {
		return true
	}
	return strings.EqualFold(string(a), string(b))
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// collectScopeNames walks the environment chain starting at env, gathering
// every value name, type alias name, and type parameter name visible from
// that point outward (including enclosing scopes and, ultimately, the
// global/built-in scope). Used to build the candidate set for spelling
// suggestions on "Cannot find name" errors.
func collectScopeNames(env *Environment) []string {
	return collectScopeNamesFor(env, true, true)
}

// collectScopeNamesFor is collectScopeNames restricted to the meanings a
// reference can have: values (variables, functions, classes, enums) and/or
// types (aliases, interfaces, type parameters). tsc only suggests candidates
// whose meaning matches the position of the unresolved name.
func collectScopeNamesFor(env *Environment, values, typeNames bool) []string {
	if env == nil {
		return nil
	}

	seen := make(map[string]bool)
	var names []string
	add := func(name string) {
		if !seen[name] {
			seen[name] = true
			names = append(names, name)
		}
	}

	for e := env; e != nil; e = e.outer {
		// Go map order is random; tsc breaks distance ties by declaration order
		// (innermost scope first). Sorting per scope at least makes the
		// choice deterministic.
		var scopeNames []string
		if values {
			for name := range e.symbols {
				// IArguments is bound as a value only so the checker can find
				// the type of `arguments`; it is an interface in tsc.
				if name == "IArguments" {
					continue
				}
				scopeNames = append(scopeNames, name)
			}
		}
		if typeNames {
			for name := range e.typeAliases {
				scopeNames = append(scopeNames, name)
			}
			for name := range e.typeParameters {
				scopeNames = append(scopeNames, name)
			}
		}
		sort.Strings(scopeNames)
		for _, name := range scopeNames {
			add(name)
		}
	}

	return names
}

// addCannotFindNameError reports that name has no binding visible from env.
// It emits TS2552 with a "Did you mean 'Y'?" suggestion when a sufficiently
// close candidate exists in scope, or plain TS2304 otherwise. This is the
// shared "Cannot find name" reporting path — use it instead of hand-rolling
// TS2304 at new call sites so spelling suggestions stay consistent.
func (c *Checker) addCannotFindNameError(node parser.Node, env *Environment, name string) {
	if !c.wantsSpellingSuggestion() {
		c.reportCannotFindName(node, name, nil)
		return
	}
	// tsc resolves a value reference with value meaning, so only values are
	// spelling candidates.
	c.reportCannotFindName(node, name, collectScopeNamesFor(env, true, false))
}

// wantsSpellingSuggestion reports whether collecting the names in scope for
// a "Did you mean" is worth it: tsc stops suggesting after 10 unresolved
// names, and a skipped library file's errors are not reported.
func (c *Checker) wantsSpellingSuggestion() bool {
	return c.nameNotFoundCount < 10 && !c.skipValidation()
}

func (c *Checker) reportCannotFindName(node parser.Node, name string, candidates []string) {
	// tsc (maximumSuggestionCount) stops computing suggestions after 10 unresolved names.
	suggestion := ""
	if c.nameNotFoundCount < 10 {
		suggestion = getSpellingSuggestion(name, candidates)
	}
	c.nameNotFoundCount++
	if suggestion != "" {
		c.addErrorWithCode(node, errors.TS2552, fmt.Sprintf("Cannot find name '%s'. Did you mean '%s'?", name, suggestion))
		return
	}
	code, msg := c.cannotFindNameDiagnostic(node, name)
	c.addErrorWithCode(node, code, msg)
}

// cannotFindNameDiagnostic mirrors tsc's getCannotFindNameDiagnosticForName:
// well-known names from node, test runners, jQuery, Bun and the DOM get a hint
// about the missing type definitions, and an unresolved shorthand property
// value is TS18004. tsc's usesWildcardTypes (`types: ["*"]`) is not modelled,
// so the "and then add it to the types field" variants are the ones used.
func (c *Checker) cannotFindNameDiagnostic(node parser.Node, name string) (string, string) {
	switch name {
	case "document", "console":
		return "TS2584", fmt.Sprintf("Cannot find name '%s'. Do you need to change your target library? Try changing the 'lib' compiler option to include 'dom'.", name)
	case "$":
		return "TS2592", fmt.Sprintf("Cannot find name '%s'. Do you need to install type definitions for jQuery? Try `npm i --save-dev @types/jquery` and then add 'jquery' to the types field in your tsconfig.", name)
	case "beforeEach", "describe", "suite", "it", "test":
		return "TS2593", fmt.Sprintf("Cannot find name '%s'. Do you need to install type definitions for a test runner? Try `npm i --save-dev @types/jest` or `npm i --save-dev @types/mocha` and then add 'jest' or 'mocha' to the types field in your tsconfig.", name)
	case "process", "require", "Buffer", "module", "NodeJS":
		return "TS2591", fmt.Sprintf("Cannot find name '%s'. Do you need to install type definitions for node? Try `npm i --save-dev @types/node` and then add 'node' to the types field in your tsconfig.", name)
	case "Bun":
		return "TS2868", fmt.Sprintf("Cannot find name '%s'. Do you need to install type definitions for Bun? Try `npm i --save-dev @types/bun` and then add 'bun' to the types field in your tsconfig.", name)
	}
	if id, ok := node.(*parser.Identifier); ok && c.shorthandRefs[id] {
		return "TS18004", fmt.Sprintf("No value exists in scope for the shorthand property '%s'. Either declare one or provide an initializer.", name)
	}
	return errors.TS2304, fmt.Sprintf("Cannot find name '%s'.", name)
}

// noteShorthandRefs records the value identifiers of node's shorthand
// properties. The parser gives a shorthand `{ x }` a value identifier that
// shares the key's token.
func (c *Checker) noteShorthandRefs(node *parser.ObjectLiteral) {
	for _, prop := range node.Properties {
		if prop == nil {
			continue
		}
		key, ok := prop.Key.(*parser.Identifier)
		if !ok || key.Token == nil {
			continue
		}
		val, ok := prop.Value.(*parser.Identifier)
		if !ok || val.Token == nil || val.Value != key.Value || val.Token.StartPos != key.Token.StartPos {
			continue
		}
		if c.shorthandRefs == nil {
			c.shorthandRefs = make(map[*parser.Identifier]bool)
		}
		c.shorthandRefs[val] = true
	}
}

// addCannotFindNamespaceError reports an unresolved name resolved with
// namespace meaning: the left side of a qualified type name (`A.B`) or of an
// import alias entity. tsc says TS2503 "Cannot find namespace", or TS2833
// with a namespace or enum in scope as the spelling suggestion.
func (c *Checker) addCannotFindNamespaceError(node parser.Node, env *Environment, name string) {
	suggestion := ""
	if c.wantsSpellingSuggestion() {
		suggestion = getSpellingSuggestion(name, collectNamespaceNames(env))
	}
	c.nameNotFoundCount++
	if suggestion != "" {
		c.addErrorWithCode(node, "TS2833", fmt.Sprintf("Cannot find namespace '%s'. Did you mean '%s'?", name, suggestion))
		return
	}
	c.addErrorWithCode(node, "TS2503", fmt.Sprintf("Cannot find namespace '%s'.", name))
}

// collectNamespaceNames lists the namespaces and enums visible from env, the
// candidates tsc offers for a name with namespace meaning.
func collectNamespaceNames(env *Environment) []string {
	seen := make(map[string]bool)
	var names []string
	for e := env; e != nil; e = e.outer {
		var scopeNames []string
		for name, t := range e.typeAliases {
			if _, ok := t.(*types.NamespaceType); ok && !seen[name] {
				scopeNames = append(scopeNames, name)
			}
		}
		for name, info := range e.symbols {
			if _, ok := info.Type.(*types.EnumType); ok && !seen[name] {
				scopeNames = append(scopeNames, name)
			}
		}
		sort.Strings(scopeNames)
		for _, name := range scopeNames {
			if !seen[name] {
				seen[name] = true
				names = append(names, name)
			}
		}
	}
	return names
}

// addCannotFindTypeNameError reports an unresolved name in a type position.
// A name that exists only as a value is TS2749 ("refers to a value, but is being
// used as a type here"); otherwise it is TS2304/TS2552 with type-meaning
// spelling candidates.
func (c *Checker) addCannotFindTypeNameError(node parser.Node, env *Environment, name string) {
	if _, _, isValue := env.Resolve(name); isValue {
		c.addErrorWithCode(node, tsValueUsedAsType, fmt.Sprintf("'%s' refers to a value, but is being used as a type here. Did you mean 'typeof %s'?", name, name))
		return
	}
	// Primitive type keywords are not symbols in tsc, so they are never suggested.
	var candidates []string
	var inScope []string
	if c.wantsSpellingSuggestion() {
		inScope = collectScopeNamesFor(env, false, true)
		// Lib generics such as Awaited are modelled outside the scope maps
		// but are global types in tsc, so they are candidates too.
		var libNames []string
		for n := range libGenericTypeNames {
			libNames = append(libNames, n)
		}
		sort.Strings(libNames)
		inScope = append(inScope, libNames...)
	}
	for _, cand := range inScope {
		switch cand {
		case "string", "number", "boolean", "null", "undefined", "void", "never",
			"unknown", "any", "object", "symbol", "bigint":
			continue
		}
		candidates = append(candidates, cand)
	}
	c.reportCannotFindName(node, name, candidates)
}

// reportPropertyNotFound reports a missing property on a member access:
// TS2339, or TS2551 with a "Did you mean" when a member of the object's type
// is close enough to the written name. typeText is how the type prints.
func (c *Checker) reportPropertyNotFound(propNode parser.Node, object parser.Expression, name, typeText string) {
	msg := fmt.Sprintf("Property '%s' does not exist on type '%s'.", name, typeText)
	if object != nil {
		if suggestion := getSpellingSuggestion(name, c.memberNames(object.GetComputedType())); suggestion != "" {
			c.addErrorWithCode(propNode, errors.TS2551, msg+fmt.Sprintf(" Did you mean '%s'?", suggestion))
			return
		}
	}
	c.addErrorWithCode(propNode, errors.TS2339, msg)
}

// memberNames lists the property names a value of type t has, for spelling
// suggestions: declared and inherited members, and the members of the
// prototype a primitive or array borrows.
func (c *Checker) memberNames(t types.Type) []string {
	if t == nil {
		return nil
	}
	t = c.apparentType(c.rel.GetWidenedType(t))
	var names []string
	add := func(m map[string]types.Type) {
		for _, name := range types.SortedPropertyNames(m) {
			if !strings.HasPrefix(name, "__COMPUTED_PROPERTY__") && !strings.HasPrefix(name, "@@") && !strings.HasPrefix(name, "#") {
				names = append(names, name)
			}
		}
	}
	protoNames := ""
	switch tt := t.(type) {
	case *types.ObjectType:
		add(tt.GetEffectiveProperties())
	case *types.ArrayType, *types.TupleType:
		protoNames = "array"
		names = append(names, "length")
	default:
		switch t {
		case types.String:
			protoNames = "string"
			names = append(names, "length")
		case types.Number:
			protoNames = "number"
		case types.Boolean:
			protoNames = "boolean"
		case types.BigInt:
			protoNames = "bigint"
		}
	}
	if protoNames != "" {
		names = append(names, c.env.PrimitivePrototypeNames(protoNames)...)
	}
	return names
}
