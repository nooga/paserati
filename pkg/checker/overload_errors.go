package checker

import (
	"fmt"
	"strings"

	"github.com/nooga/paserati/pkg/errors"
	"github.com/nooga/paserati/pkg/parser"
	"github.com/nooga/paserati/pkg/types"
)

// reportOverloadFailure reports why no overload accepted a call, following the
// way TypeScript's resolveCall chooses its diagnostic:
//   - no overload has a matching argument count: TS2575 when the count falls
//     between what overloads accept, otherwise TS2554
//   - exactly one overload has a matching count: TS2345 on its first bad argument
//   - several do: TS2769, located on the argument all of them reject, or on the
//     call when they disagree.
func (c *Checker) reportOverloadFailure(node *parser.CallExpression, argTypes []types.Type, sigs []*types.Signature) {
	n := len(argTypes)
	noOverloadMessage := "No overload matches this call." + c.overloadFailureDetails(argTypes, sigs)
	for _, arg := range node.Arguments {
		if _, isSpread := arg.(*parser.SpreadElement); isSpread {
			c.addErrorWithCode(node, errors.TS2769, noOverloadMessage)
			return
		}
	}

	var withArity []*types.Signature
	for _, sig := range sigs {
		if n >= requiredParameterCount(sig) && (sig.IsVariadic || n <= len(sig.ParameterTypes)) {
			withArity = append(withArity, sig)
		}
	}

	if len(withArity) == 0 {
		c.reportOverloadArityError(node, n, sigs)
		return
	}

	firstMismatch := func(sig *types.Signature) int { return c.firstArgumentMismatch(sig, argTypes) }

	if len(withArity) == 1 {
		sig := withArity[0]
		if i := firstMismatch(sig); i >= 0 && i < len(sig.ParameterTypes) {
			c.addErrorWithCode(node.Arguments[i], errors.TS2345, fmt.Sprintf(
				"Argument of type '%s' is not assignable to parameter of type '%s'.", argTypes[i].String(), sig.ParameterTypes[i].String()))
			return
		}
		c.addErrorWithCode(node, errors.TS2769, noOverloadMessage)
		return
	}

	if len(withArity) > 3 {
		if i := firstMismatch(withArity[len(withArity)-1]); i >= 0 {
			c.addErrorWithCode(node.Arguments[i], errors.TS2769, noOverloadMessage)
			return
		}
		c.addErrorWithCode(node, errors.TS2769, noOverloadMessage)
		return
	}

	common := -2
	for _, sig := range withArity {
		i := firstMismatch(sig)
		if common == -2 {
			common = i
		} else if common != i {
			common = -1
		}
	}
	if common >= 0 {
		c.addErrorWithCode(node.Arguments[common], errors.TS2769, noOverloadMessage)
		return
	}
	c.addErrorWithCode(node, errors.TS2769, noOverloadMessage)
}

// reportOverloadArityError mirrors TypeScript's getArgumentArityError for a set
// of overloads none of which accepts n arguments.
func (c *Checker) reportOverloadArityError(node *parser.CallExpression, n int, sigs []*types.Signature) {
	min, max := -1, -1
	hasRest := false
	for _, sig := range sigs {
		required, count := requiredParameterCount(sig), len(sig.ParameterTypes)
		if min == -1 || required < min {
			min = required
		}
		if count > max {
			max = count
		}
		if sig.IsVariadic {
			hasRest = true
		}
	}
	if min < n && n < max {
		below, above := -1, -1
		for _, sig := range sigs {
			count := len(sig.ParameterTypes)
			if count < n && count > below {
				below = count
			}
			if count > n && (above == -1 || count < above) {
				above = count
			}
		}
		c.addErrorWithCode(node, errors.TS2575, fmt.Sprintf(
			"No overload expects %d arguments, but overloads do exist that expect either %d or %d arguments.", n, below, above))
		return
	}
	expected := fmt.Sprintf("%d", min)
	if !hasRest && min < max {
		expected = fmt.Sprintf("%d-%d", min, max)
	}
	message := fmt.Sprintf("Expected %s arguments, but got %d.", expected, n)
	if hasRest {
		message = fmt.Sprintf("Expected at least %s arguments, but got %d.", expected, n)
		c.addErrorWithCode(node, errors.TS2555, message)
		return
	}
	if n > max && max < len(node.Arguments) {
		c.addErrorWithCode(node.Arguments[max], errors.TS2554, message)
		return
	}
	c.addErrorWithCode(node, errors.TS2554, message)
}

func (c *Checker) firstArgumentMismatch(sig *types.Signature, argTypes []types.Type) int {
	for i, argType := range argTypes {
		var paramType types.Type
		switch {
		case i < len(sig.ParameterTypes) && !(sig.IsVariadic && i == len(sig.ParameterTypes)-1):
			paramType = sig.ParameterTypes[i]
		case sig.RestParameterType != nil:
			if arr, ok := sig.RestParameterType.(*types.ArrayType); ok {
				paramType = arr.ElementType
			}
		}
		if paramType == nil || c.typeContainsTypeParameter(paramType) {
			continue
		}
		if !types.IsAssignable(argType, paramType) {
			return i
		}
	}
	return -1
}

// overloadFailureDetails lists, tsc-style, why each overload rejected the
// call: its signature followed by the first error it gave. Overloads whose
// failure cannot be pinned on an argument (generic signatures are not
// inferred here) are left out.
func (c *Checker) overloadFailureDetails(argTypes []types.Type, sigs []*types.Signature) string {
	n := len(argTypes)
	detail := func(sig *types.Signature) string {
		if n < requiredParameterCount(sig) || (!sig.IsVariadic && sig.RestParameterType == nil && n > len(sig.ParameterTypes)) {
			required, count := requiredParameterCount(sig), len(sig.ParameterTypes)
			expected := fmt.Sprintf("%d", required)
			if sig.RestParameterType == nil && !sig.IsVariadic && required < count {
				expected = fmt.Sprintf("%d-%d", required, count)
			}
			if sig.RestParameterType != nil || sig.IsVariadic {
				return fmt.Sprintf("Expected at least %s arguments, but got %d.", expected, n)
			}
			return fmt.Sprintf("Expected %s arguments, but got %d.", expected, n)
		}
		i := c.firstArgumentMismatch(sig, argTypes)
		if i < 0 || i >= len(sig.ParameterTypes) {
			return ""
		}
		return fmt.Sprintf("Argument of type '%s' is not assignable to parameter of type '%s'.", argTypes[i].String(), sig.ParameterTypes[i].String())
	}

	var out strings.Builder
	if len(sigs) > 3 {
		last := sigs[len(sigs)-1]
		if msg := detail(last); msg != "" {
			out.WriteString("\n  The last overload gave the following error.\n    " + msg)
		}
		return out.String()
	}
	for i, sig := range sigs {
		msg := detail(sig)
		if msg == "" {
			continue
		}
		fmt.Fprintf(&out, "\n  Overload %d of %d, '%s', gave the following error.\n    %s", i+1, len(sigs), overloadSignatureText(sig), msg)
	}
	return out.String()
}

// overloadSignatureText prints a signature the way tsc quotes it in
// diagnostics: `(name: T, other?: U): R`.
func overloadSignatureText(sig *types.Signature) string {
	var b strings.Builder
	b.WriteString("(")
	for i, p := range sig.ParameterTypes {
		if i > 0 {
			b.WriteString(", ")
		}
		name := fmt.Sprintf("arg%d", i)
		if i < len(sig.ParameterNames) && sig.ParameterNames[i] != "" {
			name = sig.ParameterNames[i]
		}
		if sig.IsVariadic && i == len(sig.ParameterTypes)-1 {
			b.WriteString("...")
		}
		b.WriteString(name)
		if i < len(sig.OptionalParams) && sig.OptionalParams[i] {
			b.WriteString("?")
		}
		b.WriteString(": ")
		if p != nil {
			b.WriteString(p.String())
		} else {
			b.WriteString("any")
		}
	}
	if sig.RestParameterType != nil {
		if len(sig.ParameterTypes) > 0 {
			b.WriteString(", ")
		}
		b.WriteString("...rest: " + sig.RestParameterType.String())
	}
	b.WriteString("): ")
	if sig.ReturnType != nil {
		b.WriteString(sig.ReturnType.String())
	} else {
		b.WriteString("void")
	}
	return b.String()
}
