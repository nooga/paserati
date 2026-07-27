package checker

import (
	"fmt"
	"strings"

	"github.com/nooga/paserati/pkg/errors"
	"github.com/nooga/paserati/pkg/parser"
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
		if v := i - int(max) - 1; v > minJ {
			minJ = v
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
		for name := range e.symbols {
			add(name)
		}
		for name := range e.typeAliases {
			add(name)
		}
		for name := range e.typeParameters {
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
	if suggestion := getSpellingSuggestion(name, collectScopeNames(env)); suggestion != "" {
		c.addErrorWithCode(node, errors.TS2552, fmt.Sprintf("Cannot find name '%s'. Did you mean '%s'?", name, suggestion))
		return
	}
	c.addErrorWithCode(node, errors.TS2304, fmt.Sprintf("Cannot find name '%s'.", name))
}
