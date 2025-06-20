package checker

import (
	"paserati/pkg/errors"
	"paserati/pkg/parser"
)

// Helper to add type errors (consider adding token/node info later)
func (c *Checker) addError(node parser.Node, message string) {
	token := parser.GetTokenFromNode(node)
	err := &errors.TypeError{
		Position: errors.Position{
			Line:     token.Line,
			Column:   token.Column,
			StartPos: token.StartPos,
			EndPos:   token.EndPos,
		},
		Msg: message,
	}
	c.errors = append(c.errors, err)
}

// Helper to add generic type errors without a specific node
func (c *Checker) addGenericError(message string) {
	err := &errors.TypeError{
		Position: errors.Position{
			Line:     1,
			Column:   1,
			StartPos: 0,
			EndPos:   0,
		},
		Msg: message,
	}
	c.errors = append(c.errors, err)
}
