package parser

import (
	"bytes"
	"fmt"
	"paserati/pkg/lexer" // Need token types
	"paserati/pkg/types" // Need types package for ComputedType
	"strings"
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
	// --- NEW: Field to store resolved type ---
	GetComputedType() types.Type
	SetComputedType(t types.Type)
}

// --- Base struct for Expressions to hold ComputedType --- (Optional but helps)
type BaseExpression struct {
	ComputedType types.Type
}

func (be *BaseExpression) GetComputedType() types.Type {
	return be.ComputedType
}

func (be *BaseExpression) SetComputedType(t types.Type) {
	be.ComputedType = t
}
func (be *BaseExpression) expressionNode() {} // Implement dummy method

// --- Program Node ---

// Program is the root node of the AST.
type Program struct {
	Statements          []Statement
	HoistedDeclarations map[string]Expression // Changed: Store hoisted Expression (e.g., FunctionLiteral)
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
	TypeAnnotation Expression  // Parsed type node (e.g., *Identifier)
	Value          Expression  // The expression being assigned
	ComputedType   types.Type  // Stores the resolved type from TypeAnnotation
}

func (ls *LetStatement) statementNode()       {}
func (ls *LetStatement) TokenLiteral() string { return ls.Token.Literal }
func (ls *LetStatement) String() string {
	var out bytes.Buffer
	out.WriteString(ls.TokenLiteral() + " ")
	if ls.Name != nil {
		out.WriteString(ls.Name.String())
	}
	if ls.TypeAnnotation != nil {
		out.WriteString(": ")
		out.WriteString(ls.TypeAnnotation.String())
	}
	out.WriteString(" = ")
	if ls.Value != nil {
		out.WriteString(ls.Value.String())
	} else {
		// Indicate undefined if no value is provided (affects computed type later)
		// out.WriteString("undefined")
	}
	if ls.ComputedType != nil {
		out.WriteString(fmt.Sprintf(" /* type: %s */", ls.ComputedType.String()))
	}
	out.WriteString(";")
	return out.String()
}

// VarStatement represents a `var` variable declaration.
// var <Name> : <TypeAnnotation> = <Value>;
// Structurally identical to LetStatement, but semantically different (hoisting).
type VarStatement struct {
	Token          lexer.Token // The lexer.VAR token
	Name           *Identifier // The variable name
	TypeAnnotation Expression  // Parsed type node (e.g., *Identifier)
	Value          Expression  // The expression being assigned
	ComputedType   types.Type  // Stores the resolved type from TypeAnnotation or Value
}

func (vs *VarStatement) statementNode()       {}
func (vs *VarStatement) TokenLiteral() string { return vs.Token.Literal }
func (vs *VarStatement) String() string {
	var out bytes.Buffer
	out.WriteString(vs.TokenLiteral() + " ")
	if vs.Name != nil {
		out.WriteString(vs.Name.String())
	}
	if vs.TypeAnnotation != nil {
		out.WriteString(": ")
		out.WriteString(vs.TypeAnnotation.String())
	}
	out.WriteString(" = ")
	if vs.Value != nil {
		out.WriteString(vs.Value.String())
	} else {
		// Indicate undefined if no value is provided (affects computed type later)
		// out.WriteString("undefined")
	}
	if vs.ComputedType != nil {
		out.WriteString(fmt.Sprintf(" /* type: %s */", vs.ComputedType.String()))
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
	TypeAnnotation Expression  // Parsed type node
	Value          Expression  // The expression being assigned
	ComputedType   types.Type  // Stores the resolved type from TypeAnnotation
}

func (cs *ConstStatement) statementNode()       {}
func (cs *ConstStatement) TokenLiteral() string { return cs.Token.Literal }
func (cs *ConstStatement) String() string {
	var out bytes.Buffer
	out.WriteString(cs.TokenLiteral() + " ")
	out.WriteString(cs.Name.String())
	if cs.TypeAnnotation != nil {
		out.WriteString(": ")
		out.WriteString(cs.TypeAnnotation.String())
	}
	out.WriteString(" = ")
	if cs.Value != nil {
		out.WriteString(cs.Value.String())
	}
	if cs.ComputedType != nil {
		out.WriteString(fmt.Sprintf(" /* type: %s */", cs.ComputedType.String()))
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
		str := es.Expression.String()
		if compType := es.Expression.GetComputedType(); compType != nil {
			str += fmt.Sprintf(" /* type: %s */", compType.String())
		}
		return str
	}
	return ""
}

// --- Expression Nodes ---

// Identifier represents an identifier in the source code.
type Identifier struct {
	BaseExpression // Embed base for ComputedType
	Token          lexer.Token
	Value          string // The name of the identifier
	IsConstant     bool   // Populated by Type Checker
}

func (i *Identifier) expressionNode()      {}
func (i *Identifier) TokenLiteral() string { return i.Token.Literal }
func (i *Identifier) String() string       { return i.Value }

// --- NEW: Parameter Node ---
// Represents a function parameter with an optional type annotation.
// <Name> : <TypeAnnotation>
type Parameter struct {
	Token          lexer.Token // The token of the parameter name
	Name           *Identifier
	TypeAnnotation Expression // Parsed type node (e.g., *Identifier)
	ComputedType   types.Type // Stores the resolved type from TypeAnnotation
}

func (p *Parameter) expressionNode()      {} // Parameters can appear in type expressions
func (p *Parameter) TokenLiteral() string { return p.Token.Literal }
func (p *Parameter) String() string {
	var out bytes.Buffer
	if p.Name != nil {
		out.WriteString(p.Name.String())
	}
	if p.TypeAnnotation != nil {
		out.WriteString(": ")
		out.WriteString(p.TypeAnnotation.String())
	}
	if p.ComputedType != nil {
		out.WriteString(fmt.Sprintf(" /* type: %s */", p.ComputedType.String()))
	}
	return out.String()
}

// BooleanLiteral represents `true` or `false`.
type BooleanLiteral struct {
	BaseExpression             // Embed base for ComputedType
	Token          lexer.Token // The lexer.TRUE or lexer.FALSE token
	Value          bool
}

func (b *BooleanLiteral) expressionNode()      {}
func (b *BooleanLiteral) TokenLiteral() string { return b.Token.Literal }
func (b *BooleanLiteral) String() string       { return b.Token.Literal }

// NumberLiteral represents numeric literals (integers or floats).
type NumberLiteral struct {
	BaseExpression             // Embed base for ComputedType
	Token          lexer.Token // The lexer.NUMBER token
	Value          float64     // Store as float64 for simplicity
}

func (n *NumberLiteral) expressionNode()      {}
func (n *NumberLiteral) TokenLiteral() string { return n.Token.Literal }
func (n *NumberLiteral) String() string       { return n.Token.Literal }

// StringLiteral represents string literals.
type StringLiteral struct {
	BaseExpression             // Embed base for ComputedType
	Token          lexer.Token // The lexer.STRING token
	Value          string
}

func (s *StringLiteral) expressionNode()      {}
func (s *StringLiteral) TokenLiteral() string { return s.Token.Literal }
func (s *StringLiteral) String() string       { return s.Token.Literal } // Maybe add quotes?

// TemplateLiteral represents template literals with interpolations.
// `hello ${name} world` becomes: ["hello ", Expression("name"), " world"]
type TemplateLiteral struct {
	BaseExpression             // Embed base for ComputedType (always string)
	Token          lexer.Token // The opening '`' token
	Parts          []Node      // Alternating string parts and expressions
}

func (tl *TemplateLiteral) expressionNode()      {}
func (tl *TemplateLiteral) TokenLiteral() string { return tl.Token.Literal }
func (tl *TemplateLiteral) String() string {
	var out bytes.Buffer
	out.WriteString("`")
	for i, part := range tl.Parts {
		if i%2 == 0 {
			// String part - escape backticks and dollar signs
			str := part.String()
			str = strings.ReplaceAll(str, "`", "\\`")
			str = strings.ReplaceAll(str, "$", "\\$")
			out.WriteString(str)
		} else {
			// Expression part
			out.WriteString("${")
			out.WriteString(part.String())
			out.WriteString("}")
		}
	}
	out.WriteString("`")
	if tl.ComputedType != nil {
		out.WriteString(fmt.Sprintf(" /* type: %s */", tl.ComputedType.String()))
	}
	return out.String()
}

// --- ADDED: Helper struct for template string parts ---
type TemplateStringPart struct {
	Value string // The actual string content
}

func (tsp *TemplateStringPart) TokenLiteral() string { return tsp.Value }
func (tsp *TemplateStringPart) String() string       { return tsp.Value }

// NullLiteral represents the `null` keyword.
type NullLiteral struct {
	BaseExpression             // Embed base for ComputedType
	Token          lexer.Token // The lexer.NULL token
}

func (nl *NullLiteral) expressionNode()      {}
func (nl *NullLiteral) TokenLiteral() string { return nl.Token.Literal }
func (nl *NullLiteral) String() string       { return nl.Token.Literal }

// UndefinedLiteral represents the `undefined` keyword.
type UndefinedLiteral struct {
	BaseExpression             // Embed base for ComputedType
	Token          lexer.Token // The lexer.UNDEFINED token
}

func (ul *UndefinedLiteral) expressionNode()      {}
func (ul *UndefinedLiteral) TokenLiteral() string { return ul.Token.Literal }
func (ul *UndefinedLiteral) String() string       { return ul.Token.Literal }

// ThisExpression represents the `this` keyword.
type ThisExpression struct {
	BaseExpression             // Embed base for ComputedType
	Token          lexer.Token // The lexer.THIS token
}

func (te *ThisExpression) expressionNode()      {}
func (te *ThisExpression) TokenLiteral() string { return te.Token.Literal }
func (te *ThisExpression) String() string       { return "this" }

// FunctionLiteral represents a function definition.
// function <Name>(<Parameters>) : <ReturnTypeAnnotation> { <Body> }
// Or anonymous: function(<Parameters>) : <ReturnTypeAnnotation> { <Body> }
type FunctionLiteral struct {
	BaseExpression                       // Embed base for ComputedType (Function type)
	Token                lexer.Token     // The 'function' token
	Name                 *Identifier     // Optional function name
	Parameters           []*Parameter    // << MODIFIED
	ReturnTypeAnnotation Expression      // << RENAMED & TYPE CHANGED
	Body                 *BlockStatement // Function body
}

func (fl *FunctionLiteral) expressionNode()      {} // Functions can be expressions
func (fl *FunctionLiteral) TokenLiteral() string { return fl.Token.Literal }
func (fl *FunctionLiteral) String() string {
	var out bytes.Buffer
	params := []string{}
	for _, p := range fl.Parameters {
		if p != nil {
			params = append(params, p.String())
		}
	}
	out.WriteString(fl.TokenLiteral())
	if fl.Name != nil {
		out.WriteString(" ")
		out.WriteString(fl.Name.String())
	}
	out.WriteString("(")
	out.WriteString(strings.Join(params, ", "))
	out.WriteString(")")
	if fl.ReturnTypeAnnotation != nil {
		out.WriteString(": ")
		out.WriteString(fl.ReturnTypeAnnotation.String())
	}
	if fl.ComputedType != nil {
		out.WriteString(fmt.Sprintf(" /* type: %s */", fl.ComputedType.String()))
	}
	out.WriteString(" ")
	if fl.Body != nil {
		out.WriteString(fl.Body.String())
	}
	return out.String()
}

// AssignmentExpression represents assignment (e.g., x = 5).
// Note: For now, only assignment to identifiers is supported.
// <Left Expression (Identifier)> = <Value Expression>
type AssignmentExpression struct {
	BaseExpression             // Embed base for ComputedType (usually type of Value)
	Token          lexer.Token // The assignment token (e.g., '=', '+=')
	Operator       string      // The operator literal (e.g., "=", "+=")
	Left           Expression  // The target of the assignment (must be Identifier for now)
	Value          Expression  // The value being assigned
}

func (ae *AssignmentExpression) expressionNode()      {}
func (ae *AssignmentExpression) TokenLiteral() string { return ae.Token.Literal }
func (ae *AssignmentExpression) String() string {
	var out bytes.Buffer
	out.WriteString("(")
	out.WriteString(ae.Left.String())
	out.WriteString(" " + ae.Operator + " ")
	out.WriteString(ae.Value.String())
	out.WriteString(")")
	if ae.ComputedType != nil {
		out.WriteString(fmt.Sprintf(" /* type: %s */", ae.ComputedType.String()))
	}
	return out.String()
}

// UpdateExpression represents prefix or postfix increment/decrement (e.g., ++x, x--).
// Currently restricted to identifiers as arguments.
type UpdateExpression struct {
	BaseExpression             // Embed base for ComputedType (usually number)
	Token          lexer.Token // The '++' or '--' token
	Operator       string      // "++" or "--"
	Argument       Expression  // The expression being updated (e.g., Identifier)
	Prefix         bool        // true if operator is prefix (++x), false if postfix (x++)
}

func (ue *UpdateExpression) expressionNode()      {}
func (ue *UpdateExpression) TokenLiteral() string { return ue.Token.Literal }
func (ue *UpdateExpression) String() string {
	var out bytes.Buffer
	if ue.Prefix {
		out.WriteString(ue.Operator)
		out.WriteString(ue.Argument.String())
	} else {
		out.WriteString(ue.Argument.String())
		out.WriteString(ue.Operator)
	}
	if ue.ComputedType != nil {
		out.WriteString(fmt.Sprintf(" /* type: %s */", ue.ComputedType.String()))
	}
	return out.String()
}

// ArrowFunctionLiteral represents an arrow function definition.
// (<Parameters>) => <BodyExpression | BodyStatements>
type ArrowFunctionLiteral struct {
	BaseExpression                    // Embed base for ComputedType (Function type)
	Token                lexer.Token  // The '=>' token
	Parameters           []*Parameter // << MODIFIED
	ReturnTypeAnnotation Expression   // << MODIFIED
	Body                 Node         // Can be Expression or *BlockStatement
}

func (afl *ArrowFunctionLiteral) expressionNode()      {}
func (afl *ArrowFunctionLiteral) TokenLiteral() string { return afl.Token.Literal }
func (afl *ArrowFunctionLiteral) String() string {
	var out bytes.Buffer
	params := []string{}
	for _, p := range afl.Parameters {
		if p != nil {
			params = append(params, p.String())
		}
	}

	if len(afl.Parameters) == 1 && afl.Parameters[0] != nil && afl.Parameters[0].TypeAnnotation == nil {
		out.WriteString(params[0])
	} else {
		out.WriteString("(")
		out.WriteString(strings.Join(params, ", "))
		out.WriteString(")")
	}

	if afl.ReturnTypeAnnotation != nil {
		out.WriteString(": ")
		out.WriteString(afl.ReturnTypeAnnotation.String())
	}

	out.WriteString(" => ")
	if afl.Body != nil {
		out.WriteString(afl.Body.String())
	}
	if afl.ComputedType != nil {
		out.WriteString(fmt.Sprintf(" /* type: %s */", afl.ComputedType.String()))
	}
	return out.String()
}

// BlockStatement represents a sequence of statements enclosed in braces.
// { <statement1>; <statement2>; ... }
type BlockStatement struct {
	Token               lexer.Token // The { token
	Statements          []Statement
	HoistedDeclarations map[string]Expression // Changed: Store hoisted Expression within this block
}

func (bs *BlockStatement) statementNode()       {} // Can act as a statement
func (bs *BlockStatement) TokenLiteral() string { return bs.Token.Literal }
func (bs *BlockStatement) String() string {
	var out bytes.Buffer
	out.WriteString("{\n")
	for _, s := range bs.Statements {
		if s != nil {
			lines := strings.Split(s.String(), "\n")
			for i, line := range lines {
				out.WriteString("\t" + line)
				if i < len(lines)-1 {
					out.WriteString("\n")
				}
			}
			out.WriteString("\n")
		}
	}
	out.WriteString("}")
	return out.String()
}

// IfExpression represents an if/else conditional expression.
// if (<Condition>) { <Consequence> } else { <Alternative> }
type IfExpression struct {
	BaseExpression             // Embed base for ComputedType (Union of consequence/alternative types?)
	Token          lexer.Token // The 'if' token
	Condition      Expression
	Consequence    *BlockStatement
	Alternative    *BlockStatement // Optional
}

func (ie *IfExpression) expressionNode()      {}
func (ie *IfExpression) TokenLiteral() string { return ie.Token.Literal }
func (ie *IfExpression) String() string {
	var out bytes.Buffer
	out.WriteString("if")
	out.WriteString(ie.Condition.String())
	out.WriteString(" ")
	out.WriteString(ie.Consequence.String())
	if ie.Alternative != nil {
		out.WriteString("else ")
		out.WriteString(ie.Alternative.String())
	}
	if ie.ComputedType != nil {
		out.WriteString(fmt.Sprintf(" /* type: %s */", ie.ComputedType.String()))
	}
	return out.String()
}

// --- New: WhileStatement ---

// WhileStatement represents a 'while (condition) { body }' statement.
type WhileStatement struct {
	Token     lexer.Token // The 'while' token
	Condition Expression
	Body      *BlockStatement
}

func (ws *WhileStatement) statementNode()       {}
func (ws *WhileStatement) TokenLiteral() string { return ws.Token.Literal }
func (ws *WhileStatement) String() string {
	var out bytes.Buffer
	out.WriteString("while")
	out.WriteString("(")
	if ws.Condition != nil {
		out.WriteString(ws.Condition.String())
	}
	out.WriteString(") ")
	if ws.Body != nil {
		out.WriteString(ws.Body.String())
	}
	return out.String()
}

// --- New: ForStatement ---

// ForStatement represents a C-style 'for (initializer; condition; update) { body }' statement.
// Initializer can be a LetStatement or an ExpressionStatement.
// Condition and Update are optional expressions.
type ForStatement struct {
	Token       lexer.Token // The 'for' token
	Initializer Statement   // Can be *LetStatement or *ExpressionStatement or nil
	Condition   Expression  // Can be nil
	Update      Expression  // Can be nil
	Body        *BlockStatement
}

func (fs *ForStatement) statementNode()       {}
func (fs *ForStatement) TokenLiteral() string { return fs.Token.Literal }
func (fs *ForStatement) String() string {
	var out bytes.Buffer
	out.WriteString("for (")
	if fs.Initializer != nil {
		out.WriteString(fs.Initializer.String())
	} else {
		if fs.Condition != nil || fs.Update != nil {
			out.WriteString(";")
		}
	}
	if fs.Condition != nil {
		out.WriteString(" ")
		out.WriteString(fs.Condition.String())
	}
	out.WriteString(";")
	if fs.Update != nil {
		out.WriteString(" ")
		out.WriteString(fs.Update.String())
	}
	out.WriteString(") ")
	if fs.Body != nil {
		out.WriteString(fs.Body.String())
	}
	return out.String()
}

// --- New: Break Statement ---
type BreakStatement struct {
	Token lexer.Token // The 'break' token
}

func (bs *BreakStatement) statementNode()       {}
func (bs *BreakStatement) TokenLiteral() string { return bs.Token.Literal }
func (bs *BreakStatement) String() string       { return bs.Token.Literal + ";" }

// --- New: Continue Statement ---
type ContinueStatement struct {
	Token lexer.Token // The 'continue' token
}

func (cs *ContinueStatement) statementNode()       {}
func (cs *ContinueStatement) TokenLiteral() string { return cs.Token.Literal }
func (cs *ContinueStatement) String() string       { return cs.Token.Literal + ";" }

// --- New: DoWhileStatement ---

// DoWhileStatement represents a `do { ... } while (condition);` loop.
type DoWhileStatement struct {
	Token     lexer.Token     // The 'do' token
	Body      *BlockStatement // The loop body
	Condition Expression      // The condition to check after the body
}

func (dws *DoWhileStatement) statementNode()       {}
func (dws *DoWhileStatement) TokenLiteral() string { return dws.Token.Literal }
func (dws *DoWhileStatement) String() string {
	var out bytes.Buffer
	out.WriteString("do ")
	out.WriteString(dws.Body.String())
	out.WriteString(" while (")
	out.WriteString(dws.Condition.String())
	out.WriteString(");")
	return out.String()
}

// --- TODO: Add more expression types later (Infix, Prefix, Call, If, etc.) ---

// PrefixExpression represents a prefix operator expression.
// <operator><Right>
// e.g., !true, -15
type PrefixExpression struct {
	BaseExpression             // Embed base for ComputedType
	Token          lexer.Token // The prefix token, e.g. ! or -
	Operator       string      // "!" or "-"
	Right          Expression  // The expression to the right of the operator
}

func (pe *PrefixExpression) expressionNode()      {}
func (pe *PrefixExpression) TokenLiteral() string { return pe.Token.Literal }
func (pe *PrefixExpression) String() string {
	var out bytes.Buffer
	out.WriteString("(")
	out.WriteString(pe.Operator)
	if pe.Right != nil {
		out.WriteString(pe.Right.String())
	}
	out.WriteString(")")
	if pe.ComputedType != nil {
		out.WriteString(fmt.Sprintf(" /* type: %s */", pe.ComputedType.String()))
	}
	return out.String()
}

// TypeofExpression represents a typeof operator expression.
// typeof <operand>
type TypeofExpression struct {
	BaseExpression             // Embed base for ComputedType (always string)
	Token          lexer.Token // The 'typeof' token
	Operand        Expression  // The expression whose type we want to get
}

func (te *TypeofExpression) expressionNode()      {}
func (te *TypeofExpression) TokenLiteral() string { return te.Token.Literal }
func (te *TypeofExpression) String() string {
	var out bytes.Buffer
	out.WriteString("typeof ")
	if te.Operand != nil {
		out.WriteString(te.Operand.String())
	}
	if te.ComputedType != nil {
		out.WriteString(fmt.Sprintf(" /* type: %s */", te.ComputedType.String()))
	}
	return out.String()
}

// InfixExpression represents an infix operator expression.
// <Left> <operator> <Right>
// e.g., 5 + 5, x == y
type InfixExpression struct {
	BaseExpression             // Embed base for ComputedType
	Token          lexer.Token // The operator token, e.g. +
	Left           Expression  // The expression to the left of the operator
	Operator       string      // e.g., "+", "-", "*", "/", "==", "!=", "<", ">"
	Right          Expression  // The expression to the right of the operator
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
	if ie.ComputedType != nil {
		out.WriteString(fmt.Sprintf(" /* type: %s */", ie.ComputedType.String()))
	}
	return out.String()
}

// CallExpression represents a function call.
// <Function>(<Arguments>)
// Function can be an identifier or a function literal.
type CallExpression struct {
	BaseExpression              // Embed base for ComputedType (Function's return type)
	Token          lexer.Token  // The '(' token
	Function       Expression   // Identifier or FunctionLiteral being called
	Arguments      []Expression // List of arguments
}

func (ce *CallExpression) expressionNode()      {}
func (ce *CallExpression) TokenLiteral() string { return ce.Token.Literal }
func (ce *CallExpression) String() string {
	var out bytes.Buffer
	args := []string{}
	for _, arg := range ce.Arguments {
		if arg != nil {
			args = append(args, arg.String())
		}
	}
	out.WriteString(ce.Function.String())
	out.WriteString("(")
	out.WriteString(strings.Join(args, ", "))
	out.WriteString(")")
	if ce.ComputedType != nil {
		out.WriteString(fmt.Sprintf(" /* type: %s */", ce.ComputedType.String()))
	}
	return out.String()
}

// NewExpression represents a constructor call with the `new` keyword.
// new <Constructor>(<Arguments>)
type NewExpression struct {
	BaseExpression              // Embed base for ComputedType (constructed object type)
	Token          lexer.Token  // The 'new' token
	Constructor    Expression   // Identifier or function being called as constructor
	Arguments      []Expression // List of arguments
}

func (ne *NewExpression) expressionNode()      {}
func (ne *NewExpression) TokenLiteral() string { return ne.Token.Literal }
func (ne *NewExpression) String() string {
	var out bytes.Buffer
	args := []string{}
	for _, arg := range ne.Arguments {
		if arg != nil {
			args = append(args, arg.String())
		}
	}
	out.WriteString("new ")
	out.WriteString(ne.Constructor.String())
	out.WriteString("(")
	out.WriteString(strings.Join(args, ", "))
	out.WriteString(")")
	if ne.ComputedType != nil {
		out.WriteString(fmt.Sprintf(" /* type: %s */", ne.ComputedType.String()))
	}
	return out.String()
}

// TernaryExpression represents a conditional (ternary) expression.
// <Condition> ? <Consequence> : <Alternative>
type TernaryExpression struct {
	BaseExpression             // Embed base for ComputedType (Union of consequence/alternative types?)
	Token          lexer.Token // The '?' token
	Condition      Expression
	Consequence    Expression
	Alternative    Expression
}

func (te *TernaryExpression) expressionNode()      {}
func (te *TernaryExpression) TokenLiteral() string { return te.Token.Literal }
func (te *TernaryExpression) String() string {
	var out bytes.Buffer
	out.WriteString("(")
	out.WriteString(te.Condition.String())
	out.WriteString(" ? ")
	out.WriteString(te.Consequence.String())
	out.WriteString(" : ")
	out.WriteString(te.Alternative.String())
	out.WriteString(")")
	if te.ComputedType != nil {
		out.WriteString(fmt.Sprintf(" /* type: %s */", te.ComputedType.String()))
	}
	return out.String()
}

// --- NEW: TypeAliasStatement ---

// TypeAliasStatement represents a `type Name = Type;` declaration.
type TypeAliasStatement struct {
	Token lexer.Token // The 'type' token
	Name  *Identifier // The name of the alias
	Type  Expression  // The type expression being aliased
}

func (tas *TypeAliasStatement) statementNode()       {}
func (tas *TypeAliasStatement) TokenLiteral() string { return tas.Token.Literal }
func (tas *TypeAliasStatement) String() string {
	var out bytes.Buffer
	out.WriteString(tas.TokenLiteral() + " ")
	out.WriteString(tas.Name.String())
	out.WriteString(" = ")
	out.WriteString(tas.Type.String())
	out.WriteString(";")
	return out.String()
}

// --- NEW: UnionTypeExpression ---

// UnionTypeExpression represents a union type (e.g., string | number).
// For now, just binary unions (A | B). Can be nested for more types.
type UnionTypeExpression struct {
	BaseExpression             // Embed base for ComputedType (which will be a UnionType)
	Token          lexer.Token // The '|' token
	Left           Expression  // The type expression on the left
	Right          Expression  // The type expression on the right
}

func (ute *UnionTypeExpression) expressionNode()      {}
func (ute *UnionTypeExpression) TokenLiteral() string { return ute.Token.Literal }
func (ute *UnionTypeExpression) String() string {
	var out bytes.Buffer
	out.WriteString("(")
	out.WriteString(ute.Left.String())
	out.WriteString(" | ")
	out.WriteString(ute.Right.String())
	out.WriteString(")")
	if ute.ComputedType != nil {
		out.WriteString(fmt.Sprintf(" /* type: %s */", ute.ComputedType.String()))
	}
	return out.String()
}

// --- NEW: ArrayLiteral ---

// ArrayLiteral represents an array literal expression (e.g., [1, "two"]).
type ArrayLiteral struct {
	BaseExpression             // Embed base for ComputedType (e.g., types.ArrayType)
	Token          lexer.Token // The '[' token
	Elements       []Expression
}

func (al *ArrayLiteral) expressionNode()      {}
func (al *ArrayLiteral) TokenLiteral() string { return al.Token.Literal }
func (al *ArrayLiteral) String() string {
	var out bytes.Buffer
	elements := []string{}
	for _, el := range al.Elements {
		if el != nil {
			elements = append(elements, el.String())
		}
	}
	out.WriteString("[")
	out.WriteString(strings.Join(elements, ", "))
	out.WriteString("]")
	if al.ComputedType != nil {
		out.WriteString(fmt.Sprintf(" /* type: %s */", al.ComputedType.String()))
	}
	return out.String()
}

// --- NEW: ArrayTypeExpression ---

// ArrayTypeExpression represents an array type syntax (e.g., number[]).
type ArrayTypeExpression struct {
	BaseExpression             // Embed base for ComputedType (types.ArrayType)
	Token          lexer.Token // The '[' token
	ElementType    Expression  // The type expression for the elements
}

func (ate *ArrayTypeExpression) expressionNode()      {}
func (ate *ArrayTypeExpression) TokenLiteral() string { return ate.Token.Literal }
func (ate *ArrayTypeExpression) String() string {
	var out bytes.Buffer
	out.WriteString(ate.ElementType.String())
	out.WriteString("[]")
	if ate.ComputedType != nil {
		out.WriteString(fmt.Sprintf(" /* type: %s */", ate.ComputedType.String()))
	}
	return out.String()
}

// --- NEW: IndexExpression ---

// IndexExpression represents accessing an element by index (e.g., myArray[i]).
type IndexExpression struct {
	BaseExpression             // Embed base for ComputedType (element type)
	Token          lexer.Token // The '[' token
	Left           Expression  // The expression evaluating to the array/object being indexed
	Index          Expression  // The expression evaluating to the index
}

func (ie *IndexExpression) expressionNode()      {}
func (ie *IndexExpression) TokenLiteral() string { return ie.Token.Literal }
func (ie *IndexExpression) String() string {
	var out bytes.Buffer
	out.WriteString("(")
	out.WriteString(ie.Left.String())
	out.WriteString("[")
	out.WriteString(ie.Index.String())
	out.WriteString("])")
	if ie.ComputedType != nil {
		out.WriteString(fmt.Sprintf(" /* type: %s */", ie.ComputedType.String()))
	}
	return out.String()
}

// --- NEW: MemberExpression ---

// MemberExpression represents accessing a property (e.g., object.property).
type MemberExpression struct {
	BaseExpression             // Embed base for ComputedType
	Token          lexer.Token // The '.' token
	Object         Expression  // The expression on the left (e.g., identifier, call result)
	Property       *Identifier // The identifier on the right (the property name)
}

func (me *MemberExpression) expressionNode()      {}
func (me *MemberExpression) TokenLiteral() string { return me.Token.Literal }
func (me *MemberExpression) String() string {
	var out bytes.Buffer
	out.WriteString("(")
	out.WriteString(me.Object.String())
	out.WriteString(".")
	out.WriteString(me.Property.String())
	out.WriteString(")")
	if me.ComputedType != nil {
		out.WriteString(fmt.Sprintf(" /* type: %s */", me.ComputedType.String()))
	}
	return out.String()
}

// --- NEW: Switch Statement Nodes ---

// SwitchCase represents a single case or default clause within a switch statement.
type SwitchCase struct {
	Token     lexer.Token     // The 'case' or 'default' token
	Condition Expression      // The expression to match (nil for default)
	Body      *BlockStatement // The block of statements to execute
}

// Not a full Node, but needs String() for debugging SwitchStatement.String()
func (sc *SwitchCase) String() string {
	var out bytes.Buffer
	if sc.Condition != nil {
		out.WriteString("case ")
		out.WriteString(sc.Condition.String())
	} else {
		out.WriteString("default")
	}
	out.WriteString(":\n") // Use newline for better readability
	if sc.Body != nil {
		// Indent the body for clarity
		bodyStr := sc.Body.String()
		indentedBody := strings.ReplaceAll(bodyStr, "\n", "\n  ") // Indent lines
		out.WriteString("  ")
		out.WriteString(indentedBody)
		out.WriteString("\n")
	}
	return out.String()
}

// SwitchStatement represents a switch statement.
// switch (Expression) { Case* Default? Case* }
type SwitchStatement struct {
	Token      lexer.Token   // The 'switch' token
	Expression Expression    // The expression being evaluated
	Cases      []*SwitchCase // The list of case/default clauses
}

func (ss *SwitchStatement) statementNode()       {}
func (ss *SwitchStatement) TokenLiteral() string { return ss.Token.Literal }
func (ss *SwitchStatement) String() string {
	var out bytes.Buffer
	out.WriteString("switch (")
	if ss.Expression != nil {
		out.WriteString(ss.Expression.String())
	}
	out.WriteString(") {\n")
	for _, c := range ss.Cases {
		// Indent cases slightly
		caseStr := c.String()
		indentedCase := strings.ReplaceAll(caseStr, "\n", "\n  ")
		out.WriteString("  ")
		out.WriteString(indentedCase)
	}
	out.WriteString("}")
	return out.String()
}

// ----------------------------------------------------------------------------
// Type Expressions (Used in Type Annotations and Type Aliases)
// ----------------------------------------------------------------------------

// FunctionTypeExpression represents a type like (number, string) => boolean
type FunctionTypeExpression struct {
	BaseExpression              // Embed base for ComputedType (Function type)
	Token          lexer.Token  // The '(' token starting the parameter list
	Parameters     []Expression // Slice of Expression nodes representing parameter types
	ReturnType     Expression   // Expression node for the return type
}

func (fte *FunctionTypeExpression) expressionNode()      {}
func (fte *FunctionTypeExpression) TokenLiteral() string { return fte.Token.Literal }
func (fte *FunctionTypeExpression) String() string {
	var out bytes.Buffer
	params := []string{}
	for _, p := range fte.Parameters {
		params = append(params, p.String())
	}

	out.WriteString("(")
	out.WriteString(strings.Join(params, ", "))
	out.WriteString(") => ")
	out.WriteString(fte.ReturnType.String())

	return out.String()
}

// GetComputedType satisfies the Expression interface (placeholder)
// The actual type is determined during type checking.
func (fte *FunctionTypeExpression) GetComputedType() types.Type { return nil }

// ----------------------------------------------------------------------------
// END Type Expressions
// ----------------------------------------------------------------------------

// --- NEW: ObjectProperty (Helper for ObjectLiteral) ---
// Represents a single key-value pair within an object literal.
type ObjectProperty struct {
	Key   Expression
	Value Expression
}

// String() for ObjectProperty (optional, but helpful for debugging)
func (op *ObjectProperty) String() string {
	keyStr := ""
	if op.Key != nil {
		keyStr = op.Key.String()
	}
	valStr := ""
	if op.Value != nil {
		valStr = op.Value.String()
	}
	return fmt.Sprintf("%s: %s", keyStr, valStr)
}

// --- END NEW: ObjectProperty ---

// ShorthandMethod represents a shorthand method in object literals like { method() { ... } }
type ShorthandMethod struct {
	BaseExpression                       // Embed base for ComputedType (Function type)
	Token                lexer.Token     // The identifier token (method name)
	Name                 *Identifier     // Method name
	Parameters           []*Parameter    // Method parameters
	ReturnTypeAnnotation Expression      // Optional return type annotation
	Body                 *BlockStatement // Method body
}

func (sm *ShorthandMethod) expressionNode()      {}
func (sm *ShorthandMethod) TokenLiteral() string { return sm.Token.Literal }
func (sm *ShorthandMethod) String() string {
	var out bytes.Buffer

	out.WriteString(sm.Name.String())
	out.WriteString("(")
	params := []string{}
	for _, p := range sm.Parameters {
		params = append(params, p.String())
	}
	out.WriteString(strings.Join(params, ", "))
	out.WriteString(")")

	if sm.ReturnTypeAnnotation != nil {
		out.WriteString(": ")
		out.WriteString(sm.ReturnTypeAnnotation.String())
	}

	out.WriteString(" ")
	out.WriteString(sm.Body.String())

	if sm.ComputedType != nil {
		out.WriteString(fmt.Sprintf(" /* type: %s */", sm.ComputedType.String()))
	}
	return out.String()
}

// --- END NEW: ShorthandMethod ---

// ObjectLiteral represents an object literal expression (e.g., { key: value, "str_key": 1 }).
type ObjectLiteral struct {
	BaseExpression             // Embed base for ComputedType (e.g., types.ObjectType)
	Token          lexer.Token // The '{' token
	// --- MODIFIED: Use slice instead of map to preserve order ---
	Properties []*ObjectProperty
}

func (ol *ObjectLiteral) expressionNode()      {}
func (ol *ObjectLiteral) TokenLiteral() string { return ol.Token.Literal }
func (ol *ObjectLiteral) String() string {
	var out bytes.Buffer
	propStrings := []string{}
	// --- MODIFIED: Iterate over slice ---
	for _, prop := range ol.Properties {
		propStrings = append(propStrings, prop.String())
	}

	out.WriteString("{")
	out.WriteString(strings.Join(propStrings, ", "))
	out.WriteString("}")
	if ol.ComputedType != nil {
		out.WriteString(fmt.Sprintf(" /* type: %s */", ol.ComputedType.String()))
	}
	return out.String()
}

// --- END NEW: ObjectLiteral ---

// ObjectTypeExpression represents an object type literal (e.g., { name: string; age: number }).
type ObjectTypeExpression struct {
	BaseExpression             // Embed base for ComputedType (which will be an ObjectType)
	Token          lexer.Token // The '{' token
	Properties     []*ObjectTypeProperty
}

func (ote *ObjectTypeExpression) expressionNode()      {}
func (ote *ObjectTypeExpression) TokenLiteral() string { return ote.Token.Literal }
func (ote *ObjectTypeExpression) String() string {
	var out bytes.Buffer
	propStrings := []string{}
	for _, prop := range ote.Properties {
		propStrings = append(propStrings, prop.String())
	}

	out.WriteString("{ ")
	out.WriteString(strings.Join(propStrings, "; "))
	out.WriteString(" }")
	if ote.ComputedType != nil {
		out.WriteString(fmt.Sprintf(" /* type: %s */", ote.ComputedType.String()))
	}
	return out.String()
}

// ObjectTypeProperty represents a property in an object type literal.
type ObjectTypeProperty struct {
	Name     *Identifier // Property name
	Type     Expression  // Property type annotation
	Optional bool        // Whether the property is optional (for future use)
}

func (otp *ObjectTypeProperty) String() string {
	var out bytes.Buffer
	out.WriteString(otp.Name.String())
	if otp.Optional {
		out.WriteString("?")
	}
	out.WriteString(": ")
	out.WriteString(otp.Type.String())
	return out.String()
}

// --- END NEW: ObjectTypeExpression ---

// InterfaceDeclaration represents an interface declaration.
// interface Name { property: Type; method(): ReturnType; }
type InterfaceDeclaration struct {
	Token      lexer.Token          // The 'interface' token
	Name       *Identifier          // Interface name
	Extends    []*Identifier        // Interfaces this interface extends (NEW)
	Properties []*InterfaceProperty // Interface properties/methods
}

func (id *InterfaceDeclaration) statementNode()       {}
func (id *InterfaceDeclaration) TokenLiteral() string { return id.Token.Literal }
func (id *InterfaceDeclaration) String() string {
	var out bytes.Buffer
	out.WriteString("interface ")
	out.WriteString(id.Name.String())

	// Add extends clause if present
	if len(id.Extends) > 0 {
		out.WriteString(" extends ")
		for i, ext := range id.Extends {
			if i > 0 {
				out.WriteString(", ")
			}
			out.WriteString(ext.String())
		}
	}

	out.WriteString(" {\n")
	for _, prop := range id.Properties {
		out.WriteString("  ")
		out.WriteString(prop.String())
		out.WriteString(";\n")
	}
	out.WriteString("}")
	return out.String()
}

// InterfaceProperty represents a property or method signature in an interface.
type InterfaceProperty struct {
	Name                   *Identifier // Property/method name
	Type                   Expression  // Type annotation (for properties) or function type (for methods)
	IsMethod               bool        // Whether this is a method signature
	Optional               bool        // Whether the property is optional (Name?)
	IsConstructorSignature bool        // Whether this is a constructor signature (new (): T)
}

func (ip *InterfaceProperty) String() string {
	var out bytes.Buffer
	if ip.IsConstructorSignature {
		out.WriteString("new ")
		out.WriteString(ip.Type.String()) // This should be a function type for the constructor
	} else {
		out.WriteString(ip.Name.String())
		if ip.Optional {
			out.WriteString("?")
		}
		out.WriteString(": ")
		out.WriteString(ip.Type.String())
	}
	return out.String()
}

// ConstructorTypeExpression represents a constructor type signature like `new () => T`
type ConstructorTypeExpression struct {
	BaseExpression              // Embed base for ComputedType
	Token          lexer.Token  // The 'new' token
	Parameters     []Expression // Parameter types for the constructor
	ReturnType     Expression   // The constructed type (T in `new (): T`)
}

func (cte *ConstructorTypeExpression) expressionNode()      {}
func (cte *ConstructorTypeExpression) TokenLiteral() string { return cte.Token.Literal }
func (cte *ConstructorTypeExpression) String() string {
	var out bytes.Buffer
	params := []string{}
	for _, p := range cte.Parameters {
		params = append(params, p.String())
	}

	out.WriteString("new (")
	out.WriteString(strings.Join(params, ", "))
	out.WriteString("): ")
	out.WriteString(cte.ReturnType.String())

	return out.String()
}

// --- END NEW ---
