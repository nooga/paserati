package checker

import (
	"paserati/pkg/errors"
	"paserati/pkg/lexer"
	"paserati/pkg/parser"
)

// Helper to add type errors (consider adding token/node info later)
func (c *Checker) addError(node parser.Node, message string) {
	token := GetTokenFromNode(node)
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

// --- NEW HELPER: GetTokenFromNode (Best effort) ---

// GetTokenFromNode attempts to extract the primary token associated with a parser node.
// This is useful for getting line numbers for error reporting.
// Returns the zero value of lexer.Token if no specific token can be easily extracted.
func GetTokenFromNode(node parser.Node) lexer.Token {
	switch n := node.(type) {
	// Statements (use the primary keyword/token)
	case *parser.LetStatement:
		return n.Token
	case *parser.ConstStatement:
		return n.Token
	case *parser.ReturnStatement:
		return n.Token
	case *parser.ExpressionStatement:
		return n.Token // Token of the start of the expression
	case *parser.BlockStatement:
		return n.Token // The '{' token
	case *parser.IfExpression:
		return n.Token // The 'if' token
	case *parser.WhileStatement:
		return n.Token
	case *parser.ForStatement:
		return n.Token
	case *parser.BreakStatement:
		return n.Token
	case *parser.ContinueStatement:
		return n.Token
	case *parser.DoWhileStatement:
		return n.Token
	case *parser.TypeAliasStatement:
		return n.Token
	case *parser.InterfaceDeclaration:
		return n.Token

	// Expressions (use the primary token where available)
	case *parser.Identifier:
		return n.Token
	case *parser.NumberLiteral:
		return n.Token
	case *parser.StringLiteral:
		return n.Token
	case *parser.BooleanLiteral:
		return n.Token
	case *parser.NullLiteral:
		return n.Token
	case *parser.UndefinedLiteral:
		return n.Token
	case *parser.ObjectLiteral: // <<< ADD THIS
		return n.Token // The '{' token
	case *parser.ShorthandMethod: // <<< ADD THIS
		return n.Token // The method name token
	case *parser.FunctionLiteral:
		return n.Token // The 'function' token
	case *parser.FunctionSignature:
		return n.Token // The 'function' token
	case *parser.ArrowFunctionLiteral:
		return n.Token // The '=>' token
	case *parser.PrefixExpression:
		return n.Token // The operator token
	case *parser.InfixExpression:
		return n.Token // The operator token
	case *parser.TernaryExpression:
		return n.Token // The '?' token
	case *parser.CallExpression:
		return n.Token // The '(' token
	case *parser.NewExpression:
		return n.Token // The 'new' token
	case *parser.IndexExpression:
		return n.Token // The '[' token
	case *parser.ArrayLiteral:
		return n.Token // The '[' token
	case *parser.AssignmentExpression:
		return n.Token // The operator token
	case *parser.UpdateExpression:
		return n.Token // The operator token
	// Add other expression types if they have a clear primary token

	// Add specific handling for UnionTypeExpression if needed, but it's primarily structural
	// case *parser.UnionTypeExpression: return n.Token // The '|' token?

	default:
		// Cannot easily determine a representative token
		return lexer.Token{} // Return zero value
	}
}
