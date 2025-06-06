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
