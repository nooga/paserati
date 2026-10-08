package checker

import (
	"fmt"

	"github.com/nooga/paserati/pkg/errors"
	"github.com/nooga/paserati/pkg/parser"
)

// Helper to add type errors (consider adding token/node info later)
func (c *Checker) addError(node parser.Node, message string) {
	c.addErrorWithCode(node, "", message)
}

// addErrorWithCode reports a diagnostic tagged with the TypeScript error code
// it corresponds to. Messages on coded diagnostics must match TypeScript's
// wording exactly — `paserati-testtsc -strict-errors` compares codes against
// the TypeScript baselines, and the codes are only meaningful if the diagnostic
// really is the same one. Pass "" for diagnostics with no TypeScript
// equivalent, or ones not mapped yet.
func (c *Checker) addErrorWithCode(node parser.Node, code string, message string) {
	token := parser.GetTokenFromNode(node)
	if code != "" {
		// tsc keeps one copy of identical diagnostics; re-visiting a node
		// (overload resolution, spread arguments, ...) must not duplicate them.
		key := fmt.Sprintf("%d:%s:%s", token.StartPos, code, message)
		if c.reportedErrors[key] {
			return
		}
		if c.reportedErrors == nil {
			c.reportedErrors = make(map[string]bool)
		}
		c.reportedErrors[key] = true
	}
	err := &errors.TypeError{
		Position: errors.Position{
			Line:     token.Line,
			Column:   token.Column,
			StartPos: token.StartPos,
			EndPos:   token.EndPos,
			Source:   c.source, // Use cached source from checker
		},
		Msg:       message,
		ErrorCode: code,
	}
	c.errors = append(c.errors, err)
}

// speculate runs fn and reports whether it raised no diagnostics. If it did,
// they are discarded, dedupe keys included, so a later real report of the
// same diagnostic still lands.
func (c *Checker) speculate(fn func()) bool {
	n := len(c.errors)
	fn()
	if len(c.errors) == n {
		return true
	}
	for _, e := range c.errors[n:] {
		if te, ok := e.(*errors.TypeError); ok && te.ErrorCode != "" {
			key := fmt.Sprintf("%d:%s:%s", te.Position.StartPos, te.ErrorCode, te.Msg)
			delete(c.reportedErrors, key)
			delete(c.reportedRelationDiagnostics, key)
		}
	}
	c.errors = c.errors[:n]
	return false
}

// Helper to add generic type errors without a specific node
func (c *Checker) addGenericError(message string) {
	err := &errors.TypeError{
		Position: errors.Position{
			Line:     1,
			Column:   1,
			StartPos: 0,
			EndPos:   0,
			Source:   c.source, // Use cached source from checker
		},
		Msg:       message,
		ErrorCode: errors.PS2004, // Use constraint violation error code
	}
	c.errors = append(c.errors, err)
}

// Helper to add constraint violation errors with proper code
func (c *Checker) addConstraintError(node parser.Node, message string) {
	token := parser.GetTokenFromNode(node)
	err := &errors.TypeError{
		Position: errors.Position{
			Line:     token.Line,
			Column:   token.Column,
			StartPos: token.StartPos,
			EndPos:   token.EndPos,
			Source:   c.source, // Use cached source from checker
		},
		Msg:       message,
		ErrorCode: errors.PS2004, // Constraint violation error code
	}
	c.errors = append(c.errors, err)
}

// reportGenericArity reports a type-argument arity error (TS2314/TS2707) at the
// type reference being instantiated.
func (c *Checker) reportGenericArity(code, message string) {
	if c.typeRefNode != nil {
		c.addErrorWithCode(c.typeRefNode, code, message)
		return
	}
	c.addGenericError(message)
}
