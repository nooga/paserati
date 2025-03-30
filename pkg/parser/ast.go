package parser

import (
	"bytes"
	"paserati/pkg/lexer" // Need token types
	"strings"
	// Add types package import later
)

// --- Interfaces ---

// Node is the base interface for all AST nodes.
type Node interface {
	TokenLiteral() string // Returns the literal value of the token associated with the node
	String() string       // Returns a string representation of the node (for debugging)
}

// Statement represents a statement node in the AST.
type Statement interface {
	Node
	statementNode() // Dummy method for distinguishing statement types
}

// Expression represents an expression node in the AST.
type Expression interface {
	Node
	expressionNode() // Dummy method for distinguishing expression types
}

// --- Program Node ---

// Program is the root node of the AST.
type Program struct {
	Statements []Statement
}

func (p *Program) TokenLiteral() string {
	if len(p.Statements) > 0 {
		return p.Statements[0].TokenLiteral()
	}
	return ""
}

func (p *Program) String() string {
	var out bytes.Buffer
	for _, s := range p.Statements {
		out.WriteString(s.String())
	}
	return out.String()
}

// --- Statement Nodes ---

// LetStatement represents a `let` variable declaration.
// let <Name> : <TypeAnnotation> = <Value>;
type LetStatement struct {
	Token          lexer.Token // The lexer.LET token
	Name           *Identifier // The variable name
	TypeAnnotation Expression  // Optional type annotation (Expression for now, refine later)
	Value          Expression  // The expression being assigned
}

func (ls *LetStatement) statementNode()       {}
func (ls *LetStatement) TokenLiteral() string { return ls.Token.Literal }
func (ls *LetStatement) String() string {
	var out bytes.Buffer
	out.WriteString(ls.TokenLiteral() + " ")
	out.WriteString(ls.Name.String())
	if ls.TypeAnnotation != nil {
		out.WriteString(" : ")
		out.WriteString(ls.TypeAnnotation.String())
	}
	out.WriteString(" = ")
	if ls.Value != nil {
		out.WriteString(ls.Value.String())
	}
	out.WriteString(";")
	return out.String()
}

// ConstStatement represents a `const` variable declaration.
// const <Name> : <TypeAnnotation> = <Value>;
// Note: Structurally identical to LetStatement for now, but semantically different.
type ConstStatement struct {
	Token          lexer.Token // The lexer.CONST token
	Name           *Identifier // The variable name
	TypeAnnotation Expression  // Optional type annotation
	Value          Expression  // The expression being assigned
}

func (cs *ConstStatement) statementNode()       {}
func (cs *ConstStatement) TokenLiteral() string { return cs.Token.Literal }
func (cs *ConstStatement) String() string {
	// Similar implementation to LetStatement.String()
	var out bytes.Buffer
	out.WriteString(cs.TokenLiteral() + " ")
	out.WriteString(cs.Name.String())
	if cs.TypeAnnotation != nil {
		out.WriteString(" : ")
		out.WriteString(cs.TypeAnnotation.String())
	}
	out.WriteString(" = ")
	if cs.Value != nil {
		out.WriteString(cs.Value.String())
	}
	out.WriteString(";")
	return out.String()
}

// ReturnStatement represents a `return` statement.
// return <ReturnValue>;
type ReturnStatement struct {
	Token       lexer.Token // The lexer.RETURN token
	ReturnValue Expression  // The expression to return
}

func (rs *ReturnStatement) statementNode()       {}
func (rs *ReturnStatement) TokenLiteral() string { return rs.Token.Literal }
func (rs *ReturnStatement) String() string {
	var out bytes.Buffer
	out.WriteString(rs.TokenLiteral() + " ")
	if rs.ReturnValue != nil {
		out.WriteString(rs.ReturnValue.String())
	}
	out.WriteString(";")
	return out.String()
}

// ExpressionStatement represents a statement consisting of a single expression.
// <expression>;
type ExpressionStatement struct {
	Token      lexer.Token // The first token of the expression
	Expression Expression
}

func (es *ExpressionStatement) statementNode()       {}
func (es *ExpressionStatement) TokenLiteral() string { return es.Token.Literal }
func (es *ExpressionStatement) String() string {
	if es.Expression != nil {
		return es.Expression.String() // Often doesn't need trailing semicolon in representation
	}
	return ""
}

// --- Expression Nodes ---

// Identifier represents an identifier (variable name, function name, type name).
type Identifier struct {
	Token lexer.Token // The lexer.IDENT token
	Value string      // The name of the identifier
}

func (i *Identifier) expressionNode()      {}
func (i *Identifier) TokenLiteral() string { return i.Token.Literal }
func (i *Identifier) String() string       { return i.Value }

// BooleanLiteral represents `true` or `false`.
type BooleanLiteral struct {
	Token lexer.Token // The lexer.TRUE or lexer.FALSE token
	Value bool
}

func (b *BooleanLiteral) expressionNode()      {}
func (b *BooleanLiteral) TokenLiteral() string { return b.Token.Literal }
func (b *BooleanLiteral) String() string       { return b.Token.Literal }

// NumberLiteral represents numeric literals (integers or floats).
type NumberLiteral struct {
	Token lexer.Token // The lexer.NUMBER token
	Value float64     // Store as float64 for simplicity
}

func (n *NumberLiteral) expressionNode()      {}
func (n *NumberLiteral) TokenLiteral() string { return n.Token.Literal }
func (n *NumberLiteral) String() string       { return n.Token.Literal }

// StringLiteral represents string literals.
type StringLiteral struct {
	Token lexer.Token // The lexer.STRING token
	Value string
}

func (s *StringLiteral) expressionNode()      {}
func (s *StringLiteral) TokenLiteral() string { return s.Token.Literal }
func (s *StringLiteral) String() string       { return s.Token.Literal } // Maybe add quotes?

// NullLiteral represents the `null` keyword.
type NullLiteral struct {
	Token lexer.Token // The lexer.NULL token
}

func (nl *NullLiteral) expressionNode()      {}
func (nl *NullLiteral) TokenLiteral() string { return nl.Token.Literal }
func (nl *NullLiteral) String() string       { return nl.Token.Literal }

// FunctionLiteral represents a function definition.
// function <Name>(<Parameters>) : <ReturnType> { <Body> }
// Or anonymous: function(<Parameters>) : <ReturnType> { <Body> }
type FunctionLiteral struct {
	Token      lexer.Token   // The 'function' token
	Name       *Identifier   // Optional function name
	Parameters []*Identifier // List of parameter names (Identifier nodes)
	// TODO: Add parameter types
	ReturnType Expression      // Optional return type annotation
	Body       *BlockStatement // Function body
}

func (fl *FunctionLiteral) expressionNode()      {} // Functions can be expressions
func (fl *FunctionLiteral) TokenLiteral() string { return fl.Token.Literal }
func (fl *FunctionLiteral) String() string {
	var out bytes.Buffer
	params := []string{}
	for _, p := range fl.Parameters {
		params = append(params, p.String()) // Add type later
	}
	out.WriteString(fl.TokenLiteral())
	if fl.Name != nil {
		out.WriteString(" ")
		out.WriteString(fl.Name.String())
	}
	out.WriteString("(")
	out.WriteString(strings.Join(params, ", "))
	out.WriteString(")")
	if fl.ReturnType != nil {
		out.WriteString(" : ")
		out.WriteString(fl.ReturnType.String())
	}
	out.WriteString(" ")
	out.WriteString(fl.Body.String())
	return out.String()
}

// ArrowFunctionLiteral represents an arrow function definition.
// (<Parameters>) => <BodyExpression>
// Or: (<Parameters>) => { <BodyStatements> }
// Or single param: Param => <BodyExpression | BodyStatements>
type ArrowFunctionLiteral struct {
	Token      lexer.Token   // The '=>' token
	Parameters []*Identifier // List of parameter names (Identifier nodes)
	// TODO: Add parameter types if supported
	Body Node // Can be Expression or *BlockStatement
}

func (afl *ArrowFunctionLiteral) expressionNode()      {}                           // Arrow functions are expressions
func (afl *ArrowFunctionLiteral) TokenLiteral() string { return afl.Token.Literal } // Returns "=>"
func (afl *ArrowFunctionLiteral) String() string {
	var out bytes.Buffer
	params := []string{}
	for _, p := range afl.Parameters {
		params = append(params, p.String())
	}

	// Formatting depends slightly on whether parens are required
	// Simple heuristic: if not exactly one param, use parens.
	if len(afl.Parameters) == 1 {
		out.WriteString(params[0])
	} else {
		out.WriteString("(")
		out.WriteString(strings.Join(params, ", "))
		out.WriteString(")")
	}

	out.WriteString(" => ")
	out.WriteString(afl.Body.String())

	return out.String()
}

// BlockStatement represents a sequence of statements enclosed in braces.
// { <statement1>; <statement2>; ... }
type BlockStatement struct {
	Token      lexer.Token // The { token
	Statements []Statement
}

func (bs *BlockStatement) statementNode()       {} // Can act as a statement
func (bs *BlockStatement) TokenLiteral() string { return bs.Token.Literal }
func (bs *BlockStatement) String() string {
	var out bytes.Buffer
	out.WriteString("{\n") // Start block
	for _, s := range bs.Statements {
		out.WriteString("\t" + s.String() + "\n") // Indent statements
	}
	out.WriteString("}") // End block
	return out.String()
}

// IfExpression represents an if/else conditional expression.
// if (<Condition>) { <Consequence> } else { <Alternative> }
type IfExpression struct {
	Token       lexer.Token // The 'if' token
	Condition   Expression
	Consequence *BlockStatement
	Alternative *BlockStatement // Optional
}

func (ie *IfExpression) expressionNode()      {}
func (ie *IfExpression) TokenLiteral() string { return ie.Token.Literal }
func (ie *IfExpression) String() string {
	var out bytes.Buffer
	out.WriteString("if")
	out.WriteString(ie.Condition.String()) // Might need parens around condition for clarity
	out.WriteString(" ")
	out.WriteString(ie.Consequence.String())
	if ie.Alternative != nil {
		out.WriteString("else ")
		out.WriteString(ie.Alternative.String())
	}
	return out.String()
}

// --- TODO: Add more expression types later (Infix, Prefix, Call, If, etc.) ---

// PrefixExpression represents a prefix operator expression.
// <operator><Right>
// e.g., !true, -15
type PrefixExpression struct {
	Token    lexer.Token // The prefix token, e.g. ! or -
	Operator string      // "!" or "-"
	Right    Expression  // The expression to the right of the operator
}

func (pe *PrefixExpression) expressionNode()      {}
func (pe *PrefixExpression) TokenLiteral() string { return pe.Token.Literal }
func (pe *PrefixExpression) String() string {
	var out bytes.Buffer
	out.WriteString("(")
	out.WriteString(pe.Operator)
	out.WriteString(pe.Right.String())
	out.WriteString(")")
	return out.String()
}

// InfixExpression represents an infix operator expression.
// <Left> <operator> <Right>
// e.g., 5 + 5, x == y
type InfixExpression struct {
	Token    lexer.Token // The operator token, e.g. +
	Left     Expression  // The expression to the left of the operator
	Operator string      // e.g., "+", "-", "*", "/", "==", "!=", "<", ">"
	Right    Expression  // The expression to the right of the operator
}

func (ie *InfixExpression) expressionNode()      {}
func (ie *InfixExpression) TokenLiteral() string { return ie.Token.Literal }
func (ie *InfixExpression) String() string {
	var out bytes.Buffer
	out.WriteString("(")
	out.WriteString(ie.Left.String())
	out.WriteString(" " + ie.Operator + " ")
	out.WriteString(ie.Right.String())
	out.WriteString(")")
	return out.String()
}

// CallExpression represents a function call.
// <Function>(<Arguments>)
// Function can be an identifier or a function literal.
type CallExpression struct {
	Token     lexer.Token  // The '(' token
	Function  Expression   // Identifier or FunctionLiteral being called
	Arguments []Expression // List of arguments
}

func (ce *CallExpression) expressionNode()      {}
func (ce *CallExpression) TokenLiteral() string { return ce.Token.Literal }
func (ce *CallExpression) String() string {
	var out bytes.Buffer
	args := []string{}
	for _, a := range ce.Arguments {
		args = append(args, a.String())
	}

	out.WriteString(ce.Function.String())
	out.WriteString("(")
	out.WriteString(strings.Join(args, ", "))
	out.WriteString(")")
	return out.String()
}
