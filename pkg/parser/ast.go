package parser

import (
	"bytes"
	"fmt"
	"os"
	"paserati/pkg/lexer"  // Need token types
	"paserati/pkg/source" // Need source package for SourceFile
	"paserati/pkg/types"  // Need types package for ComputedType
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
	Source              *source.SourceFile    // Source file context for error reporting
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
// Also supports destructuring patterns: ([a, b]: [number, number]) or ({x, y}: Point)
type Parameter struct {
	Token           lexer.Token // The token of the parameter name
	Name            *Identifier // For simple parameters
	Pattern         Expression  // For destructuring patterns (ArrayParameterPattern/ObjectParameterPattern)
	TypeAnnotation  Expression  // Parsed type node (e.g., *Identifier)
	ComputedType    types.Type  // Stores the resolved type from TypeAnnotation
	Optional        bool        // Whether this parameter is optional (param?)
	DefaultValue    Expression  // Default value expression (param = defaultValue)
	IsThis          bool        // Whether this is an explicit 'this' parameter
	IsDestructuring bool        // Whether this parameter uses destructuring pattern
}

func (p *Parameter) expressionNode()      {} // Parameters can appear in type expressions
func (p *Parameter) TokenLiteral() string { return p.Token.Literal }
func (p *Parameter) String() string {
	var out bytes.Buffer
	if p.IsThis {
		out.WriteString("this")
	} else if p.IsDestructuring && p.Pattern != nil {
		out.WriteString(p.Pattern.String())
	} else if p.Name != nil {
		out.WriteString(p.Name.String())
	}
	if p.Optional {
		out.WriteString("?")
	}
	if p.TypeAnnotation != nil {
		out.WriteString(": ")
		out.WriteString(p.TypeAnnotation.String())
	}
	if p.DefaultValue != nil {
		out.WriteString(" = ")
		out.WriteString(p.DefaultValue.String())
	}
	return out.String()
}

// RestParameter represents a rest parameter (...args) in function definitions
type RestParameter struct {
	BaseExpression             // Embed base for ComputedType
	Token          lexer.Token // The '...' token
	Name           *Identifier // The parameter name (e.g., 'args' in ...args)
	TypeAnnotation Expression  // Optional type annotation (e.g., 'string[]' in ...args: string[])
	ComputedType   types.Type  // Stores the resolved type (should be array type)
}

func (rp *RestParameter) expressionNode()      {}
func (rp *RestParameter) TokenLiteral() string { return rp.Token.Literal }
func (rp *RestParameter) String() string {
	var out bytes.Buffer
	out.WriteString("...")
	if rp.Name != nil {
		out.WriteString(rp.Name.String())
	}
	if rp.TypeAnnotation != nil {
		out.WriteString(": ")
		out.WriteString(rp.TypeAnnotation.String())
	}
	return out.String()
}

// --- NEW: TypeParameter Node ---
// Represents a type parameter in generic function declarations (e.g., T, U extends string, V = DefaultType)
// Used in function<T, U extends string, V = DefaultType>() syntax
type TypeParameter struct {
	BaseExpression             // Embed base for ComputedType
	Token          lexer.Token // The identifier token (e.g., 'T')
	Name           *Identifier // The type parameter name
	Constraint     Expression  // Optional constraint (e.g., 'string' in 'T extends string')
	DefaultType    Expression  // Optional default type (e.g., 'string' in 'T = string')
}

func (tp *TypeParameter) expressionNode()      {}
func (tp *TypeParameter) TokenLiteral() string { return tp.Token.Literal }
func (tp *TypeParameter) String() string {
	var out bytes.Buffer
	if tp.Name != nil {
		out.WriteString(tp.Name.Value)
	}
	if tp.Constraint != nil {
		out.WriteString(" extends ")
		out.WriteString(tp.Constraint.String())
	}
	if tp.DefaultType != nil {
		out.WriteString(" = ")
		out.WriteString(tp.DefaultType.String())
	}
	return out.String()
}

// SpreadElement represents spread syntax (...arr) in function calls and other contexts
type SpreadElement struct {
	BaseExpression             // Embed base for ComputedType
	Token          lexer.Token // The '...' token
	Argument       Expression  // The expression being spread (e.g., 'arr' in ...arr)
}

func (se *SpreadElement) expressionNode()      {}
func (se *SpreadElement) TokenLiteral() string { return se.Token.Literal }
func (se *SpreadElement) String() string {
	var out bytes.Buffer
	out.WriteString("...")
	if se.Argument != nil {
		out.WriteString(se.Argument.String())
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

// RegexLiteral represents a regular expression literal /pattern/flags.
type RegexLiteral struct {
	BaseExpression             // Embed base for ComputedType
	Token          lexer.Token // The lexer.REGEX_LITERAL token
	Pattern        string      // The pattern part (without slashes)
	Flags          string      // The flags part
}

func (rl *RegexLiteral) expressionNode()      {}
func (rl *RegexLiteral) TokenLiteral() string { return rl.Token.Literal }
func (rl *RegexLiteral) String() string       { return rl.Token.Literal }

// ThisExpression represents the `this` keyword.
type ThisExpression struct {
	BaseExpression             // Embed base for ComputedType
	Token          lexer.Token // The lexer.THIS token
}

func (te *ThisExpression) expressionNode()      {}
func (te *ThisExpression) TokenLiteral() string { return te.Token.Literal }
func (te *ThisExpression) String() string       { return "this" }

// SuperExpression represents super keyword expressions (super(), super.method())
type SuperExpression struct {
	BaseExpression             // Embed base for ComputedType
	Token          lexer.Token // The lexer.SUPER token
}

func (se *SuperExpression) expressionNode()      {}
func (se *SuperExpression) TokenLiteral() string { return se.Token.Literal }
func (se *SuperExpression) String() string       { return "super" }

// FunctionLiteral represents a function definition.
// function <Name>(<Parameters>) : <ReturnTypeAnnotation> { <Body> }
// Or anonymous: function(<Parameters>) : <ReturnTypeAnnotation> { <Body> }
type FunctionLiteral struct {
	BaseExpression                        // Embed base for ComputedType (Function type)
	Token                lexer.Token      // The 'function' token
	Name                 *Identifier      // Optional function name
	TypeParameters       []*TypeParameter // Generic type parameters (e.g., <T, U>)
	Parameters           []*Parameter     // Regular parameters
	RestParameter        *RestParameter   // Optional rest parameter (...args)
	ReturnTypeAnnotation Expression       // << RENAMED & TYPE CHANGED
	Body                 *BlockStatement  // Function body
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
	if fl.RestParameter != nil {
		params = append(params, fl.RestParameter.String())
	}
	out.WriteString(fl.TokenLiteral())
	if fl.Name != nil {
		out.WriteString(" ")
		out.WriteString(fl.Name.String())
	}

	// Add type parameters if present
	if len(fl.TypeParameters) > 0 {
		out.WriteString("<")
		typeParams := []string{}
		for _, tp := range fl.TypeParameters {
			if tp != nil {
				typeParams = append(typeParams, tp.String())
			}
		}
		out.WriteString(strings.Join(typeParams, ", "))
		out.WriteString(">")
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
	BaseExpression                        // Embed base for ComputedType (Function type)
	Token                lexer.Token      // The '=>' token
	TypeParameters       []*TypeParameter // Generic type parameters (e.g., <T, U>)
	Parameters           []*Parameter     // Regular parameters
	RestParameter        *RestParameter   // Optional rest parameter (...args)
	ReturnTypeAnnotation Expression       // << MODIFIED
	Body                 Node             // Can be Expression or *BlockStatement
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
	if afl.RestParameter != nil {
		params = append(params, afl.RestParameter.String())
	}

	// Add type parameters if present
	if len(afl.TypeParameters) > 0 {
		out.WriteString("<")
		typeParams := []string{}
		for _, tp := range afl.TypeParameters {
			if tp != nil {
				typeParams = append(typeParams, tp.String())
			}
		}
		out.WriteString(strings.Join(typeParams, ", "))
		out.WriteString(">")
	}

	// If we have type parameters, always use parentheses
	// Otherwise, use the existing logic for single parameter optimization
	if len(afl.TypeParameters) > 0 || len(afl.Parameters) != 1 || afl.Parameters[0] == nil || afl.Parameters[0].TypeAnnotation != nil || afl.RestParameter != nil {
		out.WriteString("(")
		out.WriteString(strings.Join(params, ", "))
		out.WriteString(")")
	} else {
		out.WriteString(params[0])
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

// --- New: IfStatement ---

// IfStatement represents a 'if (condition) { consequence } else { alternative }' statement.
type IfStatement struct {
	Token       lexer.Token // The 'if' token
	Condition   Expression
	Consequence *BlockStatement
	Alternative *BlockStatement // Optional
}

func (is *IfStatement) statementNode()       {}
func (is *IfStatement) TokenLiteral() string { return is.Token.Literal }
func (is *IfStatement) String() string {
	var out bytes.Buffer
	out.WriteString("if")
	out.WriteString("(")
	if is.Condition != nil {
		out.WriteString(is.Condition.String())
	}
	out.WriteString(") ")
	if is.Consequence != nil {
		out.WriteString(is.Consequence.String())
	}
	if is.Alternative != nil {
		out.WriteString("else ")
		out.WriteString(is.Alternative.String())
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

// --- New: ForOfStatement ---

// ForOfStatement represents a 'for (<variable> of <iterable>) { body }' statement.
// Variable can be either a *Identifier (e.g., for (item of items)) or
// a variable declaration like *LetStatement (e.g., for (let item of items))
type ForOfStatement struct {
	Token    lexer.Token     // The 'for' token
	Variable Statement       // Can be *LetStatement or *ConstStatement or *ExpressionStatement with *Identifier
	Iterable Expression      // The expression being iterated over (array, string, etc.)
	Body     *BlockStatement // The loop body
}

func (fos *ForOfStatement) statementNode()       {}
func (fos *ForOfStatement) TokenLiteral() string { return fos.Token.Literal }
func (fos *ForOfStatement) String() string {
	var out bytes.Buffer
	out.WriteString("for (")
	if fos.Variable != nil {
		out.WriteString(fos.Variable.String())
	}
	out.WriteString(" of ")
	if fos.Iterable != nil {
		out.WriteString(fos.Iterable.String())
	}
	out.WriteString(") ")
	if fos.Body != nil {
		out.WriteString(fos.Body.String())
	}
	return out.String()
}

// --- New: ForInStatement ---

// ForInStatement represents a 'for (<variable> in <object>) { body }' statement.
// Variable can be either a *Identifier (e.g., for (key in obj)) or
// a variable declaration like *LetStatement (e.g., for (let key in obj))
type ForInStatement struct {
	Token    lexer.Token     // The 'for' token
	Variable Statement       // Can be *LetStatement or *ConstStatement or *ExpressionStatement with *Identifier
	Object   Expression      // The object being iterated over
	Body     *BlockStatement // The loop body
}

func (fis *ForInStatement) statementNode()       {}
func (fis *ForInStatement) TokenLiteral() string { return fis.Token.Literal }
func (fis *ForInStatement) String() string {
	var out bytes.Buffer
	out.WriteString("for (")
	if fis.Variable != nil {
		out.WriteString(fis.Variable.String())
	}
	out.WriteString(" in ")
	if fis.Object != nil {
		out.WriteString(fis.Object.String())
	}
	out.WriteString(") ")
	if fis.Body != nil {
		out.WriteString(fis.Body.String())
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

// --- Exception Handling Statements ---

// TryStatement represents a try/catch/finally block.
type TryStatement struct {
	Token        lexer.Token     // The 'try' token
	Body         *BlockStatement // The try block
	CatchClause  *CatchClause    // Optional catch clause
	FinallyBlock *BlockStatement // Optional finally block (Phase 3)
}

func (ts *TryStatement) statementNode()       {}
func (ts *TryStatement) TokenLiteral() string { return ts.Token.Literal }
func (ts *TryStatement) String() string {
	var out bytes.Buffer
	out.WriteString("try ")
	out.WriteString(ts.Body.String())
	if ts.CatchClause != nil {
		out.WriteString(" ")
		out.WriteString(ts.CatchClause.String())
	}
	if ts.FinallyBlock != nil {
		out.WriteString(" finally ")
		out.WriteString(ts.FinallyBlock.String())
	}
	return out.String()
}

// CatchClause represents a catch block.
type CatchClause struct {
	Token     lexer.Token     // The 'catch' token
	Parameter *Identifier     // Exception variable (optional in ES2019+)
	Body      *BlockStatement // The catch block
}

func (cc *CatchClause) String() string {
	var out bytes.Buffer
	out.WriteString("catch")
	if cc.Parameter != nil {
		out.WriteString(" (")
		out.WriteString(cc.Parameter.String())
		out.WriteString(")")
	}
	out.WriteString(" ")
	out.WriteString(cc.Body.String())
	return out.String()
}

// ThrowStatement represents a throw statement.
type ThrowStatement struct {
	Token lexer.Token // The 'throw' token
	Value Expression  // The expression to throw
}

func (ths *ThrowStatement) statementNode()       {}
func (ths *ThrowStatement) TokenLiteral() string { return ths.Token.Literal }
func (ths *ThrowStatement) String() string {
	var out bytes.Buffer
	out.WriteString(ths.Token.Literal)
	out.WriteString(" ")
	if ths.Value != nil {
		out.WriteString(ths.Value.String())
	}
	out.WriteString(";")
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

// TypeAssertionExpression represents a type assertion expression (value as Type)
type TypeAssertionExpression struct {
	BaseExpression             // Embed base for ComputedType
	Token          lexer.Token // The 'as' token
	Expression     Expression  // The expression being asserted
	TargetType     Expression  // The target type annotation
}

func (tae *TypeAssertionExpression) expressionNode()      {}
func (tae *TypeAssertionExpression) TokenLiteral() string { return tae.Token.Literal }
func (tae *TypeAssertionExpression) String() string {
	var out bytes.Buffer
	out.WriteString("(")
	if tae.Expression != nil {
		out.WriteString(tae.Expression.String())
	}
	out.WriteString(" as ")
	if tae.TargetType != nil {
		out.WriteString(tae.TargetType.String())
	}
	out.WriteString(")")
	if tae.ComputedType != nil {
		out.WriteString(fmt.Sprintf(" /* type: %s */", tae.ComputedType.String()))
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
	TypeArguments  []Expression // Type arguments (e.g., <string, number>)
	Arguments      []Expression // List of arguments
}

func (ce *CallExpression) expressionNode()      {}
func (ce *CallExpression) TokenLiteral() string { return ce.Token.Literal }
func (ce *CallExpression) String() string {
	var out bytes.Buffer

	out.WriteString(ce.Function.String())

	// Add type arguments if present
	if len(ce.TypeArguments) > 0 {
		out.WriteString("<")
		for i, arg := range ce.TypeArguments {
			if i > 0 {
				out.WriteString(", ")
			}
			out.WriteString(arg.String())
		}
		out.WriteString(">")
	}

	args := []string{}
	for _, arg := range ce.Arguments {
		if arg != nil {
			args = append(args, arg.String())
		}
	}
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
	TypeArguments  []Expression // Type arguments (e.g., <string, number>)
	Arguments      []Expression // List of arguments
}

func (ne *NewExpression) expressionNode()      {}
func (ne *NewExpression) TokenLiteral() string { return ne.Token.Literal }
func (ne *NewExpression) String() string {
	var out bytes.Buffer

	out.WriteString("new ")
	out.WriteString(ne.Constructor.String())

	// Add type arguments if present
	if len(ne.TypeArguments) > 0 {
		out.WriteString("<")
		for i, arg := range ne.TypeArguments {
			if i > 0 {
				out.WriteString(", ")
			}
			out.WriteString(arg.String())
		}
		out.WriteString(">")
	}

	args := []string{}
	for _, arg := range ne.Arguments {
		if arg != nil {
			args = append(args, arg.String())
		}
	}
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
	Token          lexer.Token      // The 'type' token
	Name           *Identifier      // The name of the alias
	TypeParameters []*TypeParameter // Generic type parameters (e.g., <T, U>)
	Type           Expression       // The type expression being aliased
}

func (tas *TypeAliasStatement) statementNode()       {}
func (tas *TypeAliasStatement) TokenLiteral() string { return tas.Token.Literal }
func (tas *TypeAliasStatement) String() string {
	var out bytes.Buffer
	out.WriteString(tas.TokenLiteral() + " ")
	out.WriteString(tas.Name.String())

	// Add type parameters if present
	if len(tas.TypeParameters) > 0 {
		out.WriteString("<")
		for i, tp := range tas.TypeParameters {
			if i > 0 {
				out.WriteString(", ")
			}
			out.WriteString(tp.String())
		}
		out.WriteString(">")
	}

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

// --- NEW: IntersectionTypeExpression ---

// IntersectionTypeExpression represents an intersection type (e.g., A & B).
// For now, just binary intersections (A & B). Can be nested for more types.
type IntersectionTypeExpression struct {
	BaseExpression             // Embed base for ComputedType (which will be an IntersectionType)
	Token          lexer.Token // The '&' token
	Left           Expression  // The type expression on the left
	Right          Expression  // The type expression on the right
}

func (ite *IntersectionTypeExpression) expressionNode()      {}
func (ite *IntersectionTypeExpression) TokenLiteral() string { return ite.Token.Literal }
func (ite *IntersectionTypeExpression) String() string {
	var out bytes.Buffer
	out.WriteString("(")
	out.WriteString(ite.Left.String())
	out.WriteString(" & ")
	out.WriteString(ite.Right.String())
	out.WriteString(")")
	if ite.ComputedType != nil {
		out.WriteString(fmt.Sprintf(" /* type: %s */", ite.ComputedType.String()))
	}
	return out.String()
}

// --- NEW: GenericTypeRef ---

// GenericTypeRef represents a generic type reference (e.g., Array<string>, Promise<number>).
type GenericTypeRef struct {
	BaseExpression              // Embed base for ComputedType
	Token          lexer.Token  // The identifier token
	Name           *Identifier  // The generic type name (e.g., "Array")
	TypeArguments  []Expression // The type arguments (e.g., [string] in Array<string>)
}

func (g *GenericTypeRef) expressionNode()      {}
func (g *GenericTypeRef) TokenLiteral() string { return g.Token.Literal }
func (g *GenericTypeRef) String() string {
	var out bytes.Buffer
	out.WriteString(g.Name.Value)
	out.WriteString("<")
	args := []string{}
	for _, arg := range g.TypeArguments {
		args = append(args, arg.String())
	}
	out.WriteString(strings.Join(args, ", "))
	out.WriteString(">")
	if g.ComputedType != nil {
		out.WriteString(fmt.Sprintf(" /* type: %s */", g.ComputedType.String()))
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

// --- NEW: TupleTypeExpression ---

// TupleTypeExpression represents a tuple type syntax (e.g., [string, number, boolean?]).
type TupleTypeExpression struct {
	BaseExpression              // Embed base for ComputedType (types.TupleType)
	Token          lexer.Token  // The '[' token
	ElementTypes   []Expression // The type expressions for each element
	OptionalFlags  []bool       // Which elements are optional (same length as ElementTypes)
	RestElement    Expression   // Optional rest element type (...T[])
}

func (tte *TupleTypeExpression) expressionNode()      {}
func (tte *TupleTypeExpression) TokenLiteral() string { return tte.Token.Literal }
func (tte *TupleTypeExpression) String() string {
	var out bytes.Buffer
	out.WriteString("[")

	for i, elemType := range tte.ElementTypes {
		if elemType != nil {
			out.WriteString(elemType.String())
		} else {
			out.WriteString("<nil>")
		}

		// Add optional marker if this element is optional
		if i < len(tte.OptionalFlags) && tte.OptionalFlags[i] {
			out.WriteString("?")
		}

		if i < len(tte.ElementTypes)-1 {
			out.WriteString(", ")
		}
	}

	// Add rest element if present
	if tte.RestElement != nil {
		if len(tte.ElementTypes) > 0 {
			out.WriteString(", ")
		}
		out.WriteString("...")
		out.WriteString(tte.RestElement.String())
	}

	out.WriteString("]")
	if tte.ComputedType != nil {
		out.WriteString(fmt.Sprintf(" /* type: %s */", tte.ComputedType.String()))
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
	Property       Expression  // The property access (identifier or computed expression)
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

// OptionalChainingExpression represents optional chaining property access (e.g., object?.property).
type OptionalChainingExpression struct {
	BaseExpression             // Embed base for ComputedType
	Token          lexer.Token // The '?.' token
	Object         Expression  // The expression on the left (e.g., identifier, call result)
	Property       Expression  // The property access (identifier or computed expression)
}

func (oce *OptionalChainingExpression) expressionNode()      {}
func (oce *OptionalChainingExpression) TokenLiteral() string { return oce.Token.Literal }
func (oce *OptionalChainingExpression) String() string {
	var out bytes.Buffer
	out.WriteString("(")
	out.WriteString(oce.Object.String())
	out.WriteString("?.")
	out.WriteString(oce.Property.String())
	out.WriteString(")")
	if oce.ComputedType != nil {
		out.WriteString(fmt.Sprintf(" /* type: %s */", oce.ComputedType.String()))
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
// Module System: Import/Export Statements
// ----------------------------------------------------------------------------

// ImportDeclaration represents an import statement
// import defaultImport from "module"
// import * as name from "module"
// import { export1, export2 } from "module"
// import { export1 as alias1 } from "module"
// import defaultImport, { export1, export2 } from "module"
// import defaultImport, * as name from "module"
type ImportDeclaration struct {
	Token       lexer.Token          // The 'import' token
	Specifiers  []ImportSpecifier    // What to import (default, named, namespace)
	Source      *StringLiteral       // From where ("./module")
	IsTypeOnly  bool                 // true for "import type" statements
}

func (id *ImportDeclaration) statementNode()       {}
func (id *ImportDeclaration) TokenLiteral() string { return id.Token.Literal }
func (id *ImportDeclaration) String() string {
	var out bytes.Buffer
	out.WriteString("import ")
	if id.IsTypeOnly {
		out.WriteString("type ")
	}
	
	if len(id.Specifiers) > 0 {
		specStrs := make([]string, len(id.Specifiers))
		for i, spec := range id.Specifiers {
			specStrs[i] = spec.String()
		}
		
		// Group specifiers by type for cleaner output
		hasDefault := false
		hasNamed := false
		hasNamespace := false
		
		for _, spec := range id.Specifiers {
			switch spec.(type) {
			case *ImportDefaultSpecifier:
				hasDefault = true
			case *ImportNamedSpecifier:
				hasNamed = true
			case *ImportNamespaceSpecifier:
				hasNamespace = true
			}
		}
		
		// Output in TypeScript order: default, named, namespace
		parts := []string{}
		for _, spec := range id.Specifiers {
			if _, ok := spec.(*ImportDefaultSpecifier); ok && hasDefault {
				parts = append(parts, spec.String())
				hasDefault = false // Only add once
			}
		}
		
		if hasNamed {
			namedParts := []string{}
			for _, spec := range id.Specifiers {
				if named, ok := spec.(*ImportNamedSpecifier); ok {
					namedParts = append(namedParts, named.String())
				}
			}
			if len(namedParts) > 0 {
				parts = append(parts, "{ "+strings.Join(namedParts, ", ")+" }")
			}
		}
		
		for _, spec := range id.Specifiers {
			if _, ok := spec.(*ImportNamespaceSpecifier); ok && hasNamespace {
				parts = append(parts, spec.String())
				hasNamespace = false // Only add once
			}
		}
		
		out.WriteString(strings.Join(parts, ", "))
	}
	
	if id.Source != nil {
		out.WriteString(" from ")
		out.WriteString(id.Source.String())
	}
	out.WriteString(";")
	return out.String()
}

// ImportSpecifier is the interface for different import specifier types
type ImportSpecifier interface {
	Node
	importSpecifier() // Marker method
}

// ImportDefaultSpecifier represents: import defaultName from "module"
type ImportDefaultSpecifier struct {
	Token lexer.Token // The identifier token
	Local *Identifier // Local binding name
}

func (ids *ImportDefaultSpecifier) importSpecifier()      {}
func (ids *ImportDefaultSpecifier) TokenLiteral() string  { return ids.Token.Literal }
func (ids *ImportDefaultSpecifier) String() string        { return ids.Local.String() }

// ImportNamedSpecifier represents: import { name } or import { name as alias }
type ImportNamedSpecifier struct {
	Token      lexer.Token // The imported name token
	Imported   *Identifier // Original export name
	Local      *Identifier // Local binding name (same as Imported if no alias)
	IsTypeOnly bool        // true for "import { type name }" syntax
}

func (ins *ImportNamedSpecifier) importSpecifier()      {}
func (ins *ImportNamedSpecifier) TokenLiteral() string  { return ins.Token.Literal }
func (ins *ImportNamedSpecifier) String() string {
	if ins.Imported.Value != ins.Local.Value {
		return ins.Imported.String() + " as " + ins.Local.String()
	}
	return ins.Imported.String()
}

// ImportNamespaceSpecifier represents: import * as name from "module"
type ImportNamespaceSpecifier struct {
	Token lexer.Token // The '*' token
	Local *Identifier // Local binding name
}

func (ins *ImportNamespaceSpecifier) importSpecifier()      {}
func (ins *ImportNamespaceSpecifier) TokenLiteral() string  { return ins.Token.Literal }
func (ins *ImportNamespaceSpecifier) String() string        { return "* as " + ins.Local.String() }

// ExportDeclaration is the interface for different export declaration types
type ExportDeclaration interface {
	Statement
	exportDeclaration() // Marker method
}

// ExportNamedDeclaration represents various named export forms:
// export const x = 1;
// export function foo() {}
// export { name1, name2 };
// export { name1 as alias1 };
// export { name1 } from "module";
type ExportNamedDeclaration struct {
	Token       lexer.Token        // The 'export' token
	Declaration Statement          // Direct export: export const x = 1
	Specifiers  []ExportSpecifier  // Named exports: export { x, y }
	Source      *StringLiteral     // Re-export source: export { x } from "mod"
	IsTypeOnly  bool               // true for "export type" statements
}

func (end *ExportNamedDeclaration) statementNode()        {}
func (end *ExportNamedDeclaration) exportDeclaration()    {}
func (end *ExportNamedDeclaration) TokenLiteral() string  { return end.Token.Literal }
func (end *ExportNamedDeclaration) String() string {
	var out bytes.Buffer
	out.WriteString("export ")
	if end.IsTypeOnly {
		out.WriteString("type ")
	}
	
	if end.Declaration != nil {
		// Direct export: export const x = 1;
		out.WriteString(end.Declaration.String())
	} else if len(end.Specifiers) > 0 {
		// Named exports: export { x, y }
		out.WriteString("{ ")
		specStrs := make([]string, len(end.Specifiers))
		for i, spec := range end.Specifiers {
			specStrs[i] = spec.String()
		}
		out.WriteString(strings.Join(specStrs, ", "))
		out.WriteString(" }")
		
		if end.Source != nil {
			out.WriteString(" from ")
			out.WriteString(end.Source.String())
		}
		out.WriteString(";")
	}
	
	return out.String()
}

// ExportDefaultDeclaration represents: export default expression
type ExportDefaultDeclaration struct {
	Token       lexer.Token // The 'export' token
	Declaration Expression  // The default export expression
}

func (edd *ExportDefaultDeclaration) statementNode()        {}
func (edd *ExportDefaultDeclaration) exportDeclaration()    {}
func (edd *ExportDefaultDeclaration) TokenLiteral() string  { return edd.Token.Literal }
func (edd *ExportDefaultDeclaration) String() string {
	var out bytes.Buffer
	out.WriteString("export default ")
	if edd.Declaration != nil {
		out.WriteString(edd.Declaration.String())
	}
	out.WriteString(";")
	return out.String()
}

// ExportAllDeclaration represents: export * from "module" or export * as name from "module"
type ExportAllDeclaration struct {
	Token      lexer.Token    // The 'export' token
	Exported   *Identifier    // Optional: export * as name from "module"
	Source     *StringLiteral // The module source
	IsTypeOnly bool           // true for "export type * from" statements
}

func (ead *ExportAllDeclaration) statementNode()        {}
func (ead *ExportAllDeclaration) exportDeclaration()    {}
func (ead *ExportAllDeclaration) TokenLiteral() string  { return ead.Token.Literal }
func (ead *ExportAllDeclaration) String() string {
	var out bytes.Buffer
	out.WriteString("export ")
	if ead.IsTypeOnly {
		out.WriteString("type ")
	}
	out.WriteString("* ")
	if ead.Exported != nil {
		out.WriteString("as ")
		out.WriteString(ead.Exported.String())
		out.WriteString(" ")
	}
	if ead.Source != nil {
		out.WriteString("from ")
		out.WriteString(ead.Source.String())
	}
	out.WriteString(";")
	return out.String()
}

// ExportSpecifier represents individual export specifiers in export { ... }
type ExportSpecifier interface {
	Node
	exportSpecifier() // Marker method
}

// ExportNamedSpecifier represents: export { name } or export { name as alias }
type ExportNamedSpecifier struct {
	Token    lexer.Token // The exported name token
	Local    *Identifier // Local name being exported
	Exported *Identifier // Export name (same as Local if no alias)
}

func (ens *ExportNamedSpecifier) exportSpecifier()      {}
func (ens *ExportNamedSpecifier) TokenLiteral() string  { return ens.Token.Literal }
func (ens *ExportNamedSpecifier) String() string {
	if ens.Local.Value != ens.Exported.Value {
		return ens.Local.String() + " as " + ens.Exported.String()
	}
	return ens.Local.String()
}

// ----------------------------------------------------------------------------
// Type Expressions (Used in Type Annotations and Type Aliases)
// ----------------------------------------------------------------------------

// FunctionTypeExpression represents a type like (number, string) => boolean
type FunctionTypeExpression struct {
	BaseExpression                // Embed base for ComputedType (Function type)
	Token          lexer.Token    // The '(' token starting the parameter list
	TypeParameters []*TypeParameter // Generic type parameters (e.g., <T, U>)
	Parameters     []Expression   // Slice of Expression nodes representing parameter types
	RestParameter  Expression     // Optional rest parameter type (e.g., ...args: string[])
	ReturnType     Expression     // Expression node for the return type
}

func (fte *FunctionTypeExpression) expressionNode()      {}
func (fte *FunctionTypeExpression) TokenLiteral() string { return fte.Token.Literal }
func (fte *FunctionTypeExpression) String() string {
	var out bytes.Buffer
	
	// Add type parameters if present
	if len(fte.TypeParameters) > 0 {
		out.WriteString("<")
		for i, tp := range fte.TypeParameters {
			if i > 0 {
				out.WriteString(", ")
			}
			out.WriteString(tp.String())
		}
		out.WriteString(">")
	}
	
	params := []string{}
	for _, p := range fte.Parameters {
		params = append(params, p.String())
	}

	if fte.RestParameter != nil {
		params = append(params, "..."+fte.RestParameter.String())
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

// --- NEW: MappedTypeExpression ---

// MappedTypeExpression represents a mapped type like { [P in K]: T }
// This is used for utility types like Partial<T>, Readonly<T>, etc.
type MappedTypeExpression struct {
	BaseExpression             // Embed base for ComputedType (types.MappedType)
	Token          lexer.Token // The '{' token
	TypeParameter  *Identifier // The iteration variable (e.g., "P" in [P in K])
	ConstraintType Expression  // The type being iterated over (e.g., K in [P in K])
	ValueType      Expression  // The resulting value type for each property
	
	// Modifiers for the mapped type
	ReadonlyModifier string // "+", "-", or "" (for readonly modifier)
	OptionalModifier string // "+", "-", or "" (for optional modifier)
}

func (mte *MappedTypeExpression) expressionNode()      {}
func (mte *MappedTypeExpression) TokenLiteral() string { return mte.Token.Literal }
func (mte *MappedTypeExpression) String() string {
	var out bytes.Buffer
	
	out.WriteString("{ ")
	
	// Add modifiers
	if mte.ReadonlyModifier == "+" {
		out.WriteString("readonly ")
	} else if mte.ReadonlyModifier == "-" {
		out.WriteString("-readonly ")
	}
	
	out.WriteString("[")
	if mte.TypeParameter != nil {
		out.WriteString(mte.TypeParameter.String())
	}
	out.WriteString(" in ")
	if mte.ConstraintType != nil {
		out.WriteString(mte.ConstraintType.String())
	}
	out.WriteString("]")
	
	// Add optional modifier
	if mte.OptionalModifier == "+" {
		out.WriteString("?")
	} else if mte.OptionalModifier == "-" {
		out.WriteString("-?")
	}
	
	out.WriteString(": ")
	if mte.ValueType != nil {
		out.WriteString(mte.ValueType.String())
	}
	
	out.WriteString(" }")
	
	if mte.ComputedType != nil {
		out.WriteString(fmt.Sprintf(" /* type: %s */", mte.ComputedType.String()))
	}
	
	return out.String()
}

// GetComputedType satisfies the Expression interface (placeholder)
// The actual type is determined during type checking.
func (mte *MappedTypeExpression) GetComputedType() types.Type { return mte.ComputedType }

// --- NEW: ConditionalTypeExpression ---

// ConditionalTypeExpression represents a conditional type like T extends U ? X : Y
type ConditionalTypeExpression struct {
	BaseExpression               // Embed base for ComputedType (types.ConditionalType)
	CheckType      Expression    // The type being checked (T in T extends U ? X : Y)
	ExtendsToken   lexer.Token  // The 'extends' token
	ExtendsType    Expression    // The type being extended/checked against (U in T extends U ? X : Y)
	QuestionToken  lexer.Token  // The '?' token
	TrueType       Expression    // The type when condition is true (X in T extends U ? X : Y)
	ColonToken     lexer.Token  // The ':' token
	FalseType      Expression    // The type when condition is false (Y in T extends U ? X : Y)
}

func (cte *ConditionalTypeExpression) expressionNode()      {}
func (cte *ConditionalTypeExpression) TokenLiteral() string { return cte.ExtendsToken.Literal }
func (cte *ConditionalTypeExpression) String() string {
	var out bytes.Buffer
	
	if cte.CheckType != nil {
		out.WriteString(cte.CheckType.String())
	}
	out.WriteString(" extends ")
	if cte.ExtendsType != nil {
		out.WriteString(cte.ExtendsType.String())
	}
	out.WriteString(" ? ")
	if cte.TrueType != nil {
		out.WriteString(cte.TrueType.String())
	}
	out.WriteString(" : ")
	if cte.FalseType != nil {
		out.WriteString(cte.FalseType.String())
	}
	
	if cte.ComputedType != nil {
		out.WriteString(fmt.Sprintf(" /* type: %s */", cte.ComputedType.String()))
	}
	
	return out.String()
}

// GetComputedType satisfies the Expression interface
func (cte *ConditionalTypeExpression) GetComputedType() types.Type { return cte.ComputedType }

// --- NEW: TemplateLiteralTypeExpression ---

// TemplateLiteralTypeExpression represents a template literal type like `Hello ${T}!`
type TemplateLiteralTypeExpression struct {
	BaseExpression             // Embed base for ComputedType (types.TemplateLiteralType)
	Token          lexer.Token // The opening '`' token
	Parts          []Node      // Alternating string parts and type expressions
}

func (tlte *TemplateLiteralTypeExpression) expressionNode()      {}
func (tlte *TemplateLiteralTypeExpression) TokenLiteral() string { return tlte.Token.Literal }
func (tlte *TemplateLiteralTypeExpression) String() string {
	var out bytes.Buffer
	out.WriteString("`")
	for i, part := range tlte.Parts {
		if i%2 == 0 {
			// String part - escape backticks and dollar signs
			str := part.String()
			str = strings.ReplaceAll(str, "`", "\\`")
			str = strings.ReplaceAll(str, "$", "\\$")
			out.WriteString(str)
		} else {
			// Type expression part
			out.WriteString("${")
			out.WriteString(part.String())
			out.WriteString("}")
		}
	}
	out.WriteString("`")
	if tlte.ComputedType != nil {
		out.WriteString(fmt.Sprintf(" /* type: %s */", tlte.ComputedType.String()))
	}
	return out.String()
}

// GetComputedType satisfies the Expression interface
func (tlte *TemplateLiteralTypeExpression) GetComputedType() types.Type { return tlte.ComputedType }

// --- NEW: KeyofTypeExpression ---

// KeyofTypeExpression represents a keyof type operator like keyof T
type KeyofTypeExpression struct {
	BaseExpression             // Embed base for ComputedType
	Token          lexer.Token // The 'keyof' token
	Type           Expression  // The type to get keys from
}

func (kte *KeyofTypeExpression) expressionNode()      {}
func (kte *KeyofTypeExpression) TokenLiteral() string { return kte.Token.Literal }
func (kte *KeyofTypeExpression) String() string {
	var out bytes.Buffer
	
	out.WriteString("keyof ")
	if kte.Type != nil {
		out.WriteString(kte.Type.String())
	}
	
	if kte.ComputedType != nil {
		out.WriteString(fmt.Sprintf(" /* type: %s */", kte.ComputedType.String()))
	}
	
	return out.String()
}

// GetComputedType satisfies the Expression interface (placeholder)
// The actual type is determined during type checking.
func (kte *KeyofTypeExpression) GetComputedType() types.Type { return kte.ComputedType }

// --- NEW: TypePredicateExpression ---

// TypePredicateExpression represents a type predicate like 'x is string'
// Used in function return types to indicate that the function is a type guard
type TypePredicateExpression struct {
	BaseExpression             // Embed base for ComputedType
	Token          lexer.Token // The 'is' token
	Parameter      *Identifier // The parameter being tested (e.g., "x" in "x is string")
	Type           Expression  // The type being tested for
}

func (tpe *TypePredicateExpression) expressionNode()      {}
func (tpe *TypePredicateExpression) TokenLiteral() string { return tpe.Token.Literal }
func (tpe *TypePredicateExpression) String() string {
	var out bytes.Buffer
	
	if tpe.Parameter != nil {
		out.WriteString(tpe.Parameter.String())
	}
	out.WriteString(" is ")
	if tpe.Type != nil {
		out.WriteString(tpe.Type.String())
	}
	
	if tpe.ComputedType != nil {
		out.WriteString(fmt.Sprintf(" /* type: %s */", tpe.ComputedType.String()))
	}
	
	return out.String()
}

// GetComputedType satisfies the Expression interface (placeholder)
// The actual type is determined during type checking.
func (tpe *TypePredicateExpression) GetComputedType() types.Type { return tpe.ComputedType }

// --- NEW: IndexedAccessTypeExpression ---

// IndexedAccessTypeExpression represents an indexed access type like T[K]
// Used to access properties of a type using a key type (e.g., Person["name"])
type IndexedAccessTypeExpression struct {
	BaseExpression             // Embed base for ComputedType
	Token          lexer.Token // The '[' token
	ObjectType     Expression  // The type being indexed into (e.g., T in T[K])
	IndexType      Expression  // The key type used for indexing (e.g., K in T[K])
}

func (iate *IndexedAccessTypeExpression) expressionNode()      {}
func (iate *IndexedAccessTypeExpression) TokenLiteral() string { return iate.Token.Literal }
func (iate *IndexedAccessTypeExpression) String() string {
	var out bytes.Buffer
	
	if iate.ObjectType != nil {
		out.WriteString(iate.ObjectType.String())
	}
	out.WriteString("[")
	if iate.IndexType != nil {
		out.WriteString(iate.IndexType.String())
	}
	out.WriteString("]")
	
	if iate.ComputedType != nil {
		out.WriteString(fmt.Sprintf(" /* type: %s */", iate.ComputedType.String()))
	}
	
	return out.String()
}

// GetComputedType satisfies the Expression interface (placeholder)
// The actual type is determined during type checking.
func (iate *IndexedAccessTypeExpression) GetComputedType() types.Type { return iate.ComputedType }

// ----------------------------------------------------------------------------
// END Type Expressions
// ----------------------------------------------------------------------------

// --- NEW: ObjectProperty (Helper for ObjectLiteral) ---
// Represents a single key-value pair within an object literal.
// For spread elements, Key will be a SpreadElement and Value will be nil.
type ObjectProperty struct {
	Key   Expression
	Value Expression
}

// String() for ObjectProperty (optional, but helpful for debugging)
func (op *ObjectProperty) String() string {
	// Check if this is a spread element
	if spreadElement, isSpread := op.Key.(*SpreadElement); isSpread {
		return fmt.Sprintf("...%s", spreadElement.Argument.String())
	}

	// Regular property
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
	Parameters           []*Parameter    // Regular method parameters
	RestParameter        *RestParameter  // Optional rest parameter (...args)
	ReturnTypeAnnotation Expression      // Optional return type annotation
	Body                 *BlockStatement // Method body
}

func (sm *ShorthandMethod) expressionNode()      {}
func (sm *ShorthandMethod) TokenLiteral() string { return sm.Token.Literal }
func (sm *ShorthandMethod) String() string {
	var out bytes.Buffer
	params := []string{}
	for _, p := range sm.Parameters {
		if p != nil {
			params = append(params, p.String())
		}
	}
	if sm.RestParameter != nil {
		params = append(params, sm.RestParameter.String())
	}

	out.WriteString(sm.Name.String())
	out.WriteString("(")
	out.WriteString(strings.Join(params, ", "))
	out.WriteString(")")

	if sm.ReturnTypeAnnotation != nil {
		out.WriteString(": ")
		out.WriteString(sm.ReturnTypeAnnotation.String())
	}

	if sm.ComputedType != nil {
		out.WriteString(fmt.Sprintf(" /* type: %s */", sm.ComputedType.String()))
	}

	out.WriteString(" ")
	if sm.Body != nil {
		out.WriteString(sm.Body.String())
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
	// --- MODIFIED: Iterate over properties ---
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
	Name            *Identifier  // Property name (nil for call signatures and index signatures)
	Type            Expression   // Property type annotation or function type for call signatures
	Optional        bool         // Whether the property is optional (for future use)
	IsCallSignature bool         // Whether this is a call signature like (param: type): returnType
	Parameters      []Expression // Parameters for call signatures (only used when IsCallSignature is true)
	ReturnType      Expression   // Return type for call signatures (only used when IsCallSignature is true)
	
	// Index signature fields
	IsIndexSignature bool       // Whether this is an index signature like [key: string]: Type
	KeyName          *Identifier // The key parameter name (e.g., "key" in [key: string]: Type)
	KeyType          Expression  // The key type (e.g., "string" in [key: string]: Type)
	ValueType        Expression  // The value type (e.g., "Type" in [key: string]: Type)
}

func (otp *ObjectTypeProperty) String() string {
	var out bytes.Buffer

	if otp.IsCallSignature {
		// Call signature: (param1: type1, param2: type2): returnType
		params := []string{}
		for _, p := range otp.Parameters {
			params = append(params, p.String())
		}
		out.WriteString("(")
		out.WriteString(strings.Join(params, ", "))
		out.WriteString("): ")
		if otp.ReturnType != nil {
			out.WriteString(otp.ReturnType.String())
		}
	} else if otp.IsIndexSignature {
		// Index signature: [key: string]: Type
		out.WriteString("[")
		if otp.KeyName != nil {
			out.WriteString(otp.KeyName.String())
		}
		out.WriteString(": ")
		if otp.KeyType != nil {
			out.WriteString(otp.KeyType.String())
		}
		out.WriteString("]: ")
		if otp.ValueType != nil {
			out.WriteString(otp.ValueType.String())
		}
	} else {
		// Regular property: name?: type
		if otp.Name != nil {
			out.WriteString(otp.Name.String())
		}
		if otp.Optional {
			out.WriteString("?")
		}
		out.WriteString(": ")
		if otp.Type != nil {
			out.WriteString(otp.Type.String())
		}
	}
	return out.String()
}

// --- END NEW: ObjectTypeExpression ---

// InterfaceDeclaration represents an interface declaration.
// interface Name { property: Type; method(): ReturnType; }
type InterfaceDeclaration struct {
	Token          lexer.Token          // The 'interface' token
	Name           *Identifier          // Interface name
	TypeParameters []*TypeParameter     // Generic type parameters (e.g., <T, U>)
	Extends        []Expression         // Interfaces this interface extends (supports generic types)
	Properties     []*InterfaceProperty // Interface properties/methods
}

func (id *InterfaceDeclaration) statementNode()       {}
func (id *InterfaceDeclaration) TokenLiteral() string { return id.Token.Literal }
func (id *InterfaceDeclaration) String() string {
	var out bytes.Buffer
	out.WriteString("interface ")
	out.WriteString(id.Name.String())

	// Add type parameters if present
	if len(id.TypeParameters) > 0 {
		out.WriteString("<")
		for i, tp := range id.TypeParameters {
			if i > 0 {
				out.WriteString(", ")
			}
			out.WriteString(tp.String())
		}
		out.WriteString(">")
	}

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
	ComputedName           Expression  // Computed property name for [expression]: syntax
	Type                   Expression  // Type annotation (for properties) or function type (for methods)
	IsMethod               bool        // Whether this is a method signature
	Optional               bool        // Whether the property is optional (Name?)
	IsConstructorSignature bool        // Whether this is a constructor signature (new (): T)
	IsComputedProperty     bool        // Whether this is a computed property name [expr]:
	
	// Index signature fields
	IsIndexSignature bool       // Whether this is an index signature like [key: string]: Type
	KeyName          *Identifier // The key parameter name (e.g., "key" in [key: string]: Type)
	KeyType          Expression  // The key type (e.g., "string" in [key: string]: Type)
	ValueType        Expression  // The value type (e.g., "Type" in [key: string]: Type)
}

func (ip *InterfaceProperty) String() string {
	var out bytes.Buffer
	if ip.IsConstructorSignature {
		out.WriteString("new ")
		out.WriteString(ip.Type.String()) // This should be a function type for the constructor
	} else if ip.IsIndexSignature {
		// Index signature: [key: string]: Type
		out.WriteString("[")
		if ip.KeyName != nil {
			out.WriteString(ip.KeyName.String())
		}
		out.WriteString(": ")
		if ip.KeyType != nil {
			out.WriteString(ip.KeyType.String())
		}
		out.WriteString("]: ")
		if ip.ValueType != nil {
			out.WriteString(ip.ValueType.String())
		}
	} else if ip.IsComputedProperty {
		// Computed property: [expr]: Type
		out.WriteString("[")
		if ip.ComputedName != nil {
			out.WriteString(ip.ComputedName.String())
		}
		out.WriteString("]")
		if ip.Optional {
			out.WriteString("?")
		}
		out.WriteString(": ")
		out.WriteString(ip.Type.String())
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

// --- NEW: Destructuring Assignment Support ---

// DestructuringElement represents a single element in destructuring pattern
type DestructuringElement struct {
	Target  Expression // Target variable (Identifier for now)
	Default Expression // Default value (nil if no default)
	IsRest  bool       // true if this is a rest element (...target)
}

// String() for DestructuringElement (helpful for debugging)
func (de *DestructuringElement) String() string {
	var out bytes.Buffer
	if de.IsRest {
		out.WriteString("...")
	}
	if de.Target != nil {
		out.WriteString(de.Target.String())
	}
	if de.Default != nil {
		out.WriteString(" = ")
		out.WriteString(de.Default.String())
	}
	return out.String()
}

// ArrayDestructuringAssignment represents [a, b, c] = expr
type ArrayDestructuringAssignment struct {
	BaseExpression                         // Embed base for ComputedType
	Token          lexer.Token             // The '[' token
	Elements       []*DestructuringElement // Target variables/patterns
	Value          Expression              // RHS expression to destructure
}

func (ada *ArrayDestructuringAssignment) expressionNode()      {}
func (ada *ArrayDestructuringAssignment) TokenLiteral() string { return ada.Token.Literal }
func (ada *ArrayDestructuringAssignment) String() string {
	var out bytes.Buffer
	elements := []string{}
	for _, el := range ada.Elements {
		if el != nil {
			elements = append(elements, el.String())
		}
	}
	out.WriteString("[")
	out.WriteString(strings.Join(elements, ", "))
	out.WriteString("] = ")
	if ada.Value != nil {
		out.WriteString(ada.Value.String())
	}
	if ada.ComputedType != nil {
		out.WriteString(fmt.Sprintf(" /* type: %s */", ada.ComputedType.String()))
	}
	return out.String()
}

// DestructuringProperty represents key: target in object destructuring
type DestructuringProperty struct {
	Key     *Identifier // Property name in source object
	Target  Expression  // Target variable (can be different from key)
	Default Expression  // Default value (nil if no default)
}

// String() for DestructuringProperty (helpful for debugging)
func (dp *DestructuringProperty) String() string {
	var out bytes.Buffer
	if dp.Key != nil {
		out.WriteString(dp.Key.String())
	}
	if dp.Target != nil && dp.Target != dp.Key {
		out.WriteString(": ")
		out.WriteString(dp.Target.String())
	}
	if dp.Default != nil {
		out.WriteString(" = ")
		out.WriteString(dp.Default.String())
	}
	return out.String()
}

// ObjectDestructuringAssignment represents {a, b} = expr
type ObjectDestructuringAssignment struct {
	BaseExpression                          // Embed base for ComputedType
	Token          lexer.Token              // The '{' token
	Properties     []*DestructuringProperty // Target properties/patterns
	RestProperty   *DestructuringElement    // Rest property (...rest) - optional
	Value          Expression               // RHS expression to destructure
}

func (oda *ObjectDestructuringAssignment) expressionNode()      {}
func (oda *ObjectDestructuringAssignment) TokenLiteral() string { return oda.Token.Literal }
func (oda *ObjectDestructuringAssignment) String() string {
	var out bytes.Buffer
	properties := []string{}
	for _, prop := range oda.Properties {
		if prop != nil {
			properties = append(properties, prop.String())
		}
	}
	if oda.RestProperty != nil {
		properties = append(properties, "..."+oda.RestProperty.String())
	}
	out.WriteString("{")
	out.WriteString(strings.Join(properties, ", "))
	out.WriteString("} = ")
	if oda.Value != nil {
		out.WriteString(oda.Value.String())
	}
	if oda.ComputedType != nil {
		out.WriteString(fmt.Sprintf(" /* type: %s */", oda.ComputedType.String()))
	}
	return out.String()
}

// --- END NEW: Destructuring Assignment Support ---

// --- NEW: Destructuring Declaration Support ---

// ArrayDestructuringDeclaration represents let/const/var [a, b, c] = expr
type ArrayDestructuringDeclaration struct {
	Token          lexer.Token             // The 'let', 'const', or 'var' token
	IsConst        bool                    // true for const, false for let/var
	Elements       []*DestructuringElement // Target variables/patterns
	TypeAnnotation Expression              // Optional type annotation (e.g., : [number, string])
	Value          Expression              // RHS expression to destructure
}

func (add *ArrayDestructuringDeclaration) statementNode()       {}
func (add *ArrayDestructuringDeclaration) TokenLiteral() string { return add.Token.Literal }
func (add *ArrayDestructuringDeclaration) String() string {
	var out bytes.Buffer
	out.WriteString(add.Token.Literal)
	out.WriteString(" ")

	elements := []string{}
	for _, el := range add.Elements {
		if el != nil {
			elements = append(elements, el.String())
		}
	}
	out.WriteString("[")
	out.WriteString(strings.Join(elements, ", "))
	out.WriteString("]")

	if add.TypeAnnotation != nil {
		out.WriteString(": ")
		out.WriteString(add.TypeAnnotation.String())
	}

	if add.Value != nil {
		out.WriteString(" = ")
		out.WriteString(add.Value.String())
	}

	return out.String()
}

// ObjectDestructuringDeclaration represents let/const/var {a, b} = expr
type ObjectDestructuringDeclaration struct {
	Token          lexer.Token              // The 'let', 'const', or 'var' token
	IsConst        bool                     // true for const, false for let/var
	Properties     []*DestructuringProperty // Target properties/patterns
	RestProperty   *DestructuringElement    // Rest property (...rest) - optional
	TypeAnnotation Expression               // Optional type annotation (e.g., : {a: number, b: string})
	Value          Expression               // RHS expression to destructure
}

func (odd *ObjectDestructuringDeclaration) statementNode()       {}
func (odd *ObjectDestructuringDeclaration) TokenLiteral() string { return odd.Token.Literal }
func (odd *ObjectDestructuringDeclaration) String() string {
	var out bytes.Buffer
	out.WriteString(odd.Token.Literal)
	out.WriteString(" ")

	properties := []string{}
	for _, prop := range odd.Properties {
		if prop != nil {
			properties = append(properties, prop.String())
		}
	}
	if odd.RestProperty != nil {
		properties = append(properties, "..."+odd.RestProperty.String())
	}
	out.WriteString("{")
	out.WriteString(strings.Join(properties, ", "))
	out.WriteString("}")

	if odd.TypeAnnotation != nil {
		out.WriteString(": ")
		out.WriteString(odd.TypeAnnotation.String())
	}

	if odd.Value != nil {
		out.WriteString(" = ")
		out.WriteString(odd.Value.String())
	}

	return out.String()
}

// --- END NEW: Destructuring Declaration Support ---

// --- NEW: Parameter Pattern Support ---

// ArrayParameterPattern represents array destructuring in function parameters
// Examples: ([a, b]: [number, number]) => {}
type ArrayParameterPattern struct {
	BaseExpression                         // Embed base for ComputedType
	Token          lexer.Token             // The '[' token
	Elements       []*DestructuringElement // Parameter elements (can have defaults and rest)
}

func (app *ArrayParameterPattern) expressionNode()      {}
func (app *ArrayParameterPattern) TokenLiteral() string { return app.Token.Literal }
func (app *ArrayParameterPattern) String() string {
	var out bytes.Buffer
	elements := []string{}
	for _, el := range app.Elements {
		if el != nil {
			elements = append(elements, el.String())
		}
	}
	out.WriteString("[")
	out.WriteString(strings.Join(elements, ", "))
	out.WriteString("]")
	return out.String()
}

// ObjectParameterPattern represents object destructuring in function parameters
// Examples: ({x, y}: Point) => {}, ({name = "Unknown"}: {name?: string}) => {}
type ObjectParameterPattern struct {
	BaseExpression                          // Embed base for ComputedType
	Token          lexer.Token              // The '{' token
	Properties     []*DestructuringProperty // Parameter properties (can have defaults)
	RestProperty   *DestructuringElement    // Rest property (...rest) - optional
}

func (opp *ObjectParameterPattern) expressionNode()      {}
func (opp *ObjectParameterPattern) TokenLiteral() string { return opp.Token.Literal }
func (opp *ObjectParameterPattern) String() string {
	if opp == nil {
		return "{<nil>}"
	}
	var out bytes.Buffer
	elements := []string{}
	for _, prop := range opp.Properties {
		if prop != nil {
			elements = append(elements, prop.String())
		}
	}
	if opp.RestProperty != nil {
		elements = append(elements, opp.RestProperty.String())
	}
	out.WriteString("{")
	out.WriteString(strings.Join(elements, ", "))
	out.WriteString("}")
	return out.String()
}

// --- END NEW: Parameter Pattern Support ---

// --- Function Overload Support ---

// FunctionSignature represents a function signature without a body (used in overloads)
type FunctionSignature struct {
	BaseExpression                      // Embed base for ComputedType (so it can be an Expression too)
	Token                lexer.Token    // The 'function' token
	Name                 *Identifier    // Function name (must match other overloads)
	Parameters           []*Parameter   // Regular function parameters with type annotations
	RestParameter        *RestParameter // Optional rest parameter (...args)
	ReturnTypeAnnotation Expression     // Return type annotation (required for overloads)
}

func (fs *FunctionSignature) statementNode()       {}
func (fs *FunctionSignature) expressionNode()      {} // NEW: Also implement Expression interface
func (fs *FunctionSignature) TokenLiteral() string { return fs.Token.Literal }
func (fs *FunctionSignature) String() string {
	var out bytes.Buffer
	params := []string{}
	for _, p := range fs.Parameters {
		if p != nil {
			params = append(params, p.String())
		}
	}
	if fs.RestParameter != nil {
		params = append(params, fs.RestParameter.String())
	}

	out.WriteString(fs.TokenLiteral())
	if fs.Name != nil {
		out.WriteString(" ")
		out.WriteString(fs.Name.String())
	}
	out.WriteString("(")
	out.WriteString(strings.Join(params, ", "))
	out.WriteString(")")
	if fs.ReturnTypeAnnotation != nil {
		out.WriteString(": ")
		out.WriteString(fs.ReturnTypeAnnotation.String())
	}
	if fs.ComputedType != nil {
		out.WriteString(fmt.Sprintf(" /* type: %s */", fs.ComputedType.String()))
	}
	out.WriteString(";")
	return out.String()
}

// FunctionOverloadGroup represents a group of function overload signatures plus an implementation
type FunctionOverloadGroup struct {
	Token          lexer.Token          // The token of the first function declaration
	Name           *Identifier          // Function name (shared by all overloads)
	Overloads      []*FunctionSignature // The overload signatures (without bodies)
	Implementation *FunctionLiteral     // The implementation (with body)
}

func (fog *FunctionOverloadGroup) statementNode()       {}
func (fog *FunctionOverloadGroup) TokenLiteral() string { return fog.Token.Literal }
func (fog *FunctionOverloadGroup) String() string {
	var out bytes.Buffer

	// Print all overload signatures
	for _, overload := range fog.Overloads {
		out.WriteString(overload.String())
		out.WriteString("\n")
	}

	// Print implementation
	if fog.Implementation != nil {
		out.WriteString(fog.Implementation.String())
	}

	return out.String()
}

// --- Class-related AST Nodes ---

// ClassDeclaration represents a class declaration statement
type ClassDeclaration struct {
	Token          lexer.Token      // The 'class' token
	Name           *Identifier      // Class name
	TypeParameters []*TypeParameter // Generic type parameters (e.g., <T, U>)
	SuperClass     Expression       // nil for basic classes (supports generic extends)
	Implements     []*Identifier    // Interfaces this class implements
	Body           *ClassBody       // Class body containing methods and properties
	IsAbstract     bool             // true if this is an abstract class
}

func (cd *ClassDeclaration) statementNode()       {}
func (cd *ClassDeclaration) TokenLiteral() string { return cd.Token.Literal }
func (cd *ClassDeclaration) String() string {
	var out bytes.Buffer
	if cd.IsAbstract {
		out.WriteString("abstract ")
	}
	out.WriteString("class ")
	if cd.Name != nil {
		out.WriteString(cd.Name.String())
	}

	// Add type parameters if present
	if len(cd.TypeParameters) > 0 {
		out.WriteString("<")
		for i, tp := range cd.TypeParameters {
			if i > 0 {
				out.WriteString(", ")
			}
			out.WriteString(tp.String())
		}
		out.WriteString(">")
	}
	if cd.SuperClass != nil {
		out.WriteString(" extends ")
		out.WriteString(cd.SuperClass.String())
	}
	if len(cd.Implements) > 0 {
		out.WriteString(" implements ")
		for i, iface := range cd.Implements {
			if i > 0 {
				out.WriteString(", ")
			}
			out.WriteString(iface.String())
		}
	}
	out.WriteString(" ")
	if cd.Body != nil {
		out.WriteString(cd.Body.String())
	}
	return out.String()
}

// ClassExpression represents a class expression (can be anonymous)
type ClassExpression struct {
	BaseExpression
	Token          lexer.Token      // The 'class' token
	Name           *Identifier      // nil for anonymous classes
	TypeParameters []*TypeParameter // Generic type parameters (e.g., <T, U>)
	SuperClass     Expression       // nil for basic classes (supports generic extends)
	Implements     []*Identifier    // Interfaces this class implements
	Body           *ClassBody       // Class body containing methods and properties
	IsAbstract     bool             // true if this is an abstract class
}

func (ce *ClassExpression) TokenLiteral() string { return ce.Token.Literal }
func (ce *ClassExpression) String() string {
	var out bytes.Buffer
	if ce.IsAbstract {
		out.WriteString("abstract ")
	}
	out.WriteString("class")
	if ce.Name != nil {
		out.WriteString(" ")
		out.WriteString(ce.Name.String())
	}

	// Add type parameters if present
	if len(ce.TypeParameters) > 0 {
		out.WriteString("<")
		for i, tp := range ce.TypeParameters {
			if i > 0 {
				out.WriteString(", ")
			}
			out.WriteString(tp.String())
		}
		out.WriteString(">")
	}
	if ce.SuperClass != nil {
		out.WriteString(" extends ")
		out.WriteString(ce.SuperClass.String())
	}
	if len(ce.Implements) > 0 {
		out.WriteString(" implements ")
		for i, iface := range ce.Implements {
			if i > 0 {
				out.WriteString(", ")
			}
			out.WriteString(iface.String())
		}
	}
	out.WriteString(" ")
	if ce.Body != nil {
		out.WriteString(ce.Body.String())
	}
	return out.String()
}

// ClassBody represents the body of a class containing methods and properties
type ClassBody struct {
	Token           lexer.Token             // The '{' token
	Methods         []*MethodDefinition     // Class method implementations
	Properties      []*PropertyDefinition   // Class properties
	ConstructorSigs []*ConstructorSignature // Constructor overload signatures
	MethodSigs      []*MethodSignature      // Method overload signatures
}

func (cb *ClassBody) TokenLiteral() string { return cb.Token.Literal }
func (cb *ClassBody) String() string {
	var out bytes.Buffer
	out.WriteString("{\n")

	for _, prop := range cb.Properties {
		out.WriteString("  ")
		out.WriteString(prop.String())
		out.WriteString("\n")
	}

	for _, method := range cb.Methods {
		out.WriteString("  ")
		out.WriteString(method.String())
		out.WriteString("\n")
	}

	out.WriteString("}")
	return out.String()
}

// MethodDefinition represents a method in a class
type MethodDefinition struct {
	BaseExpression
	Token       lexer.Token      // The method name token
	Key         Expression       // Method name (Identifier or ComputedPropertyName)
	Value       *FunctionLiteral // Function implementation
	Kind        string           // "constructor", "method"
	IsStatic    bool             // For static method support
	IsPublic    bool             // For public access modifier
	IsPrivate   bool             // For private access modifier
	IsProtected bool             // For protected access modifier
	IsAbstract  bool             // For abstract methods (no implementation)
	IsOverride  bool             // For override keyword
}

func (md *MethodDefinition) TokenLiteral() string { return md.Token.Literal }
func (md *MethodDefinition) String() string {
	var out bytes.Buffer

	// Add access modifiers
	if md.IsPrivate {
		out.WriteString("private ")
	} else if md.IsProtected {
		out.WriteString("protected ")
	} else if md.IsPublic {
		out.WriteString("public ")
	}

	if md.IsAbstract {
		out.WriteString("abstract ")
	}
	if md.IsOverride {
		out.WriteString("override ")
	}
	if md.IsStatic {
		out.WriteString("static ")
	}
	if md.Kind == "constructor" {
		out.WriteString("constructor")
	} else if md.Kind == "getter" {
		out.WriteString("get ")
		if md.Key != nil {
			out.WriteString(md.Key.String())
		}
	} else if md.Kind == "setter" {
		out.WriteString("set ")
		if md.Key != nil {
			out.WriteString(md.Key.String())
		}
	} else if md.Key != nil {
		out.WriteString(md.Key.String())
	}
	if md.Value != nil {
		// Print function signature without 'function' keyword
		out.WriteString("(")
		if md.Value.Parameters != nil {
			for i, param := range md.Value.Parameters {
				if i > 0 {
					out.WriteString(", ")
				}
				out.WriteString(param.String())
			}
		}
		out.WriteString(") ")
		if md.Value.Body != nil {
			out.WriteString(md.Value.Body.String())
		}
	}
	return out.String()
}

// ConstructorSignature represents a constructor overload signature in a class
type ConstructorSignature struct {
	Token                lexer.Token      // The 'constructor' token
	TypeParameters       []*TypeParameter // Generic type parameters (e.g., <T, U>)
	Parameters           []*Parameter     // Parameter list
	RestParameter        *RestParameter   // Rest parameter (if any)
	ReturnTypeAnnotation Expression       // Optional return type
	IsStatic             bool             // Access modifiers
	IsPublic             bool
	IsPrivate            bool
	IsProtected          bool
}

func (cs *ConstructorSignature) statementNode()       {}
func (cs *ConstructorSignature) TokenLiteral() string { return cs.Token.Literal }
func (cs *ConstructorSignature) String() string {
	var out bytes.Buffer

	// Add access modifiers
	if cs.IsPrivate {
		out.WriteString("private ")
	} else if cs.IsProtected {
		out.WriteString("protected ")
	} else if cs.IsPublic {
		out.WriteString("public ")
	}
	if cs.IsStatic {
		out.WriteString("static ")
	}

	out.WriteString("constructor(")
	if cs.Parameters != nil {
		for i, param := range cs.Parameters {
			if i > 0 {
				out.WriteString(", ")
			}
			out.WriteString(param.String())
		}
	}
	out.WriteString(")")

	if cs.ReturnTypeAnnotation != nil {
		out.WriteString(": ")
		out.WriteString(cs.ReturnTypeAnnotation.String())
	}
	out.WriteString(";")

	return out.String()
}

// MethodSignature represents a method overload signature in a class
type MethodSignature struct {
	Token                lexer.Token      // The method name token
	Key                  Expression       // Method name (Identifier or ComputedPropertyName)
	TypeParameters       []*TypeParameter // Generic type parameters (e.g., <T, U>)
	Parameters           []*Parameter     // Parameter list
	RestParameter        *RestParameter   // Rest parameter (if any)
	ReturnTypeAnnotation Expression       // Optional return type
	Kind                 string           // "method", "getter", "setter"
	IsStatic             bool             // Access modifiers
	IsPublic             bool
	IsPrivate            bool
	IsProtected          bool
	IsAbstract           bool // Abstract method modifier
	IsOverride           bool // Override method modifier
}

func (ms *MethodSignature) statementNode()       {}
func (ms *MethodSignature) TokenLiteral() string { return ms.Token.Literal }
func (ms *MethodSignature) String() string {
	var out bytes.Buffer

	// Add access modifiers
	if ms.IsPrivate {
		out.WriteString("private ")
	} else if ms.IsProtected {
		out.WriteString("protected ")
	} else if ms.IsPublic {
		out.WriteString("public ")
	}
	if ms.IsStatic {
		out.WriteString("static ")
	}
	if ms.IsAbstract {
		out.WriteString("abstract ")
	}
	if ms.IsOverride {
		out.WriteString("override ")
	}

	if ms.Kind == "getter" {
		out.WriteString("get ")
	} else if ms.Kind == "setter" {
		out.WriteString("set ")
	}

	if ms.Key != nil {
		out.WriteString(ms.Key.String())
	}

	out.WriteString("(")
	if ms.Parameters != nil {
		for i, param := range ms.Parameters {
			if i > 0 {
				out.WriteString(", ")
			}
			out.WriteString(param.String())
		}
	}
	out.WriteString(")")

	if ms.ReturnTypeAnnotation != nil {
		out.WriteString(": ")
		out.WriteString(ms.ReturnTypeAnnotation.String())
	}
	out.WriteString(";")

	return out.String()
}

// ComputedPropertyName represents a computed property name [expression]
type ComputedPropertyName struct {
	BaseExpression
	Expr Expression // The computed expression
}

func (cpn *ComputedPropertyName) TokenLiteral() string { return "[" }
func (cpn *ComputedPropertyName) String() string       { return "[" + cpn.Expr.String() + "]" }

// PropertyDefinition represents a property declaration in a class
type PropertyDefinition struct {
	BaseExpression
	Token          lexer.Token // The property name token
	Key            Expression  // Property name (Identifier or ComputedPropertyName)
	TypeAnnotation Expression  // Type annotation (can be nil)
	Value          Expression  // Initializer expression (can be nil)
	IsStatic       bool        // For static property support
	Optional       bool        // Whether the property is optional (prop?)
	Readonly       bool        // Whether the property is readonly
	IsPublic       bool        // For public access modifier
	IsPrivate      bool        // For private access modifier
	IsProtected    bool        // For protected access modifier
}

func (pd *PropertyDefinition) TokenLiteral() string { return pd.Token.Literal }
func (pd *PropertyDefinition) String() string {
	var out bytes.Buffer

	// Add access modifiers
	if pd.IsPrivate {
		out.WriteString("private ")
	} else if pd.IsProtected {
		out.WriteString("protected ")
	} else if pd.IsPublic {
		out.WriteString("public ")
	}

	if pd.Readonly {
		out.WriteString("readonly ")
	}
	if pd.IsStatic {
		out.WriteString("static ")
	}
	if pd.Key != nil {
		out.WriteString(pd.Key.String())
	}
	if pd.Optional {
		out.WriteString("?")
	}
	if pd.TypeAnnotation != nil {
		out.WriteString(": ")
		out.WriteString(pd.TypeAnnotation.String())
	}
	if pd.Value != nil {
		out.WriteString(" = ")
		out.WriteString(pd.Value.String())
	}
	out.WriteString(";")
	return out.String()
}

// --- AST Dump Utility ---

// DumpASTEnabled controls whether AST dumping is enabled
var DumpASTEnabled = false

// DumpAST prints a structured representation of the AST to stderr if enabled
func DumpAST(program *Program, title string) {
	if !DumpASTEnabled {
		return
	}

	fmt.Fprintf(os.Stderr, "\n=== AST DUMP: %s ===\n", title)
	if program == nil {
		fmt.Fprintf(os.Stderr, "Program: null\n")
		return
	}

	fmt.Fprintf(os.Stderr, "Program {\n")
	fmt.Fprintf(os.Stderr, "  statements: [\n")

	for i, stmt := range program.Statements {
		fmt.Fprintf(os.Stderr, "    [%d] ", i)
		dumpNode(stmt, "    ")
		if i < len(program.Statements)-1 {
			fmt.Fprintf(os.Stderr, ",")
		}
		fmt.Fprintf(os.Stderr, "\n")
	}

	fmt.Fprintf(os.Stderr, "  ]")

	if len(program.HoistedDeclarations) > 0 {
		fmt.Fprintf(os.Stderr, ",\n  hoistedDeclarations: {\n")
		count := 0
		for name, decl := range program.HoistedDeclarations {
			fmt.Fprintf(os.Stderr, "    %s: ", name)
			dumpNode(decl, "    ")
			if count < len(program.HoistedDeclarations)-1 {
				fmt.Fprintf(os.Stderr, ",")
			}
			fmt.Fprintf(os.Stderr, "\n")
			count++
		}
		fmt.Fprintf(os.Stderr, "  }")
	}

	fmt.Fprintf(os.Stderr, "\n}\n=== END AST DUMP ===\n\n")
}

// dumpNode prints a structured representation of an AST node
func dumpNode(node Node, indent string) {
	if node == nil {
		fmt.Fprintf(os.Stderr, "null")
		return
	}

	switch n := node.(type) {
	case *LetStatement:
		fmt.Fprintf(os.Stderr, "LetStatement {\n")
		fmt.Fprintf(os.Stderr, "%s  name: ", indent)
		dumpNode(n.Name, indent+"  ")
		fmt.Fprintf(os.Stderr, ",\n%s  typeAnnotation: ", indent)
		dumpNode(n.TypeAnnotation, indent+"  ")
		fmt.Fprintf(os.Stderr, ",\n%s  value: ", indent)
		dumpNode(n.Value, indent+"  ")
		fmt.Fprintf(os.Stderr, "\n%s}", indent)

	case *VarStatement:
		fmt.Fprintf(os.Stderr, "VarStatement {\n")
		fmt.Fprintf(os.Stderr, "%s  name: ", indent)
		dumpNode(n.Name, indent+"  ")
		fmt.Fprintf(os.Stderr, ",\n%s  typeAnnotation: ", indent)
		dumpNode(n.TypeAnnotation, indent+"  ")
		fmt.Fprintf(os.Stderr, ",\n%s  value: ", indent)
		dumpNode(n.Value, indent+"  ")
		fmt.Fprintf(os.Stderr, "\n%s}", indent)

	case *ConstStatement:
		fmt.Fprintf(os.Stderr, "ConstStatement {\n")
		fmt.Fprintf(os.Stderr, "%s  name: ", indent)
		dumpNode(n.Name, indent+"  ")
		fmt.Fprintf(os.Stderr, ",\n%s  typeAnnotation: ", indent)
		dumpNode(n.TypeAnnotation, indent+"  ")
		fmt.Fprintf(os.Stderr, ",\n%s  value: ", indent)
		dumpNode(n.Value, indent+"  ")
		fmt.Fprintf(os.Stderr, "\n%s}", indent)

	case *ExpressionStatement:
		fmt.Fprintf(os.Stderr, "ExpressionStatement {\n")
		fmt.Fprintf(os.Stderr, "%s  expression: ", indent)
		dumpNode(n.Expression, indent+"  ")
		fmt.Fprintf(os.Stderr, "\n%s}", indent)

	case *ClassDeclaration:
		fmt.Fprintf(os.Stderr, "ClassDeclaration {\n")
		fmt.Fprintf(os.Stderr, "%s  name: ", indent)
		dumpNode(n.Name, indent+"  ")
		fmt.Fprintf(os.Stderr, ",\n%s  superClass: ", indent)
		if n.SuperClass != nil {
			dumpNode(n.SuperClass, indent+"  ")
		} else {
			fmt.Fprintf(os.Stderr, "null")
		}
		fmt.Fprintf(os.Stderr, ",\n%s  body: ", indent)
		dumpNode(n.Body, indent+"  ")
		fmt.Fprintf(os.Stderr, "\n%s}", indent)

	case *ClassExpression:
		fmt.Fprintf(os.Stderr, "ClassExpression {\n")
		fmt.Fprintf(os.Stderr, "%s  name: ", indent)
		if n.Name != nil {
			dumpNode(n.Name, indent+"  ")
		} else {
			fmt.Fprintf(os.Stderr, "null")
		}
		fmt.Fprintf(os.Stderr, ",\n%s  superClass: ", indent)
		if n.SuperClass != nil {
			dumpNode(n.SuperClass, indent+"  ")
		} else {
			fmt.Fprintf(os.Stderr, "null")
		}
		fmt.Fprintf(os.Stderr, ",\n%s  body: ", indent)
		dumpNode(n.Body, indent+"  ")
		fmt.Fprintf(os.Stderr, "\n%s}", indent)

	case *ClassBody:
		fmt.Fprintf(os.Stderr, "ClassBody {\n")
		fmt.Fprintf(os.Stderr, "%s  properties: [", indent)
		for i, prop := range n.Properties {
			if i > 0 {
				fmt.Fprintf(os.Stderr, ", ")
			}
			dumpNode(prop, indent+"  ")
		}
		fmt.Fprintf(os.Stderr, "],\n%s  methods: [", indent)
		for i, method := range n.Methods {
			if i > 0 {
				fmt.Fprintf(os.Stderr, ", ")
			}
			dumpNode(method, indent+"  ")
		}
		fmt.Fprintf(os.Stderr, "]\n%s}", indent)

	case *PropertyDefinition:
		fmt.Fprintf(os.Stderr, "PropertyDefinition {\n")
		fmt.Fprintf(os.Stderr, "%s  key: ", indent)
		dumpNode(n.Key, indent+"  ")
		fmt.Fprintf(os.Stderr, ",\n%s  typeAnnotation: ", indent)
		dumpNode(n.TypeAnnotation, indent+"  ")
		fmt.Fprintf(os.Stderr, ",\n%s  value: ", indent)
		dumpNode(n.Value, indent+"  ")
		fmt.Fprintf(os.Stderr, ",\n%s  isStatic: %t", indent, n.IsStatic)
		fmt.Fprintf(os.Stderr, ",\n%s  optional: %t", indent, n.Optional)
		fmt.Fprintf(os.Stderr, ",\n%s  readonly: %t", indent, n.Readonly)
		fmt.Fprintf(os.Stderr, "\n%s}", indent)

	case *MethodDefinition:
		fmt.Fprintf(os.Stderr, "MethodDefinition {\n")
		fmt.Fprintf(os.Stderr, "%s  key: ", indent)
		dumpNode(n.Key, indent+"  ")
		fmt.Fprintf(os.Stderr, ",\n%s  kind: %q", indent, n.Kind)
		fmt.Fprintf(os.Stderr, ",\n%s  value: ", indent)
		dumpNode(n.Value, indent+"  ")
		fmt.Fprintf(os.Stderr, ",\n%s  isStatic: %t", indent, n.IsStatic)
		fmt.Fprintf(os.Stderr, "\n%s}", indent)

	case *NewExpression:
		fmt.Fprintf(os.Stderr, "NewExpression {\n")
		fmt.Fprintf(os.Stderr, "%s  constructor: ", indent)
		dumpNode(n.Constructor, indent+"  ")
		fmt.Fprintf(os.Stderr, ",\n%s  typeArguments: [", indent)
		for i, arg := range n.TypeArguments {
			if i > 0 {
				fmt.Fprintf(os.Stderr, ", ")
			}
			dumpNode(arg, indent+"  ")
		}
		fmt.Fprintf(os.Stderr, "]\n%s}", indent)

	case *CallExpression:
		fmt.Fprintf(os.Stderr, "CallExpression {\n")
		fmt.Fprintf(os.Stderr, "%s  function: ", indent)
		dumpNode(n.Function, indent+"  ")
		fmt.Fprintf(os.Stderr, ",\n%s  typeArguments: [", indent)
		for i, arg := range n.TypeArguments {
			if i > 0 {
				fmt.Fprintf(os.Stderr, ", ")
			}
			dumpNode(arg, indent+"  ")
		}
		fmt.Fprintf(os.Stderr, "],\n%s  arguments: [", indent)
		for i, arg := range n.Arguments {
			if i > 0 {
				fmt.Fprintf(os.Stderr, ", ")
			}
			dumpNode(arg, indent+"  ")
		}
		fmt.Fprintf(os.Stderr, "]\n%s}", indent)

	case *MemberExpression:
		fmt.Fprintf(os.Stderr, "MemberExpression {\n")
		fmt.Fprintf(os.Stderr, "%s  object: ", indent)
		dumpNode(n.Object, indent+"  ")
		fmt.Fprintf(os.Stderr, ",\n%s  property: ", indent)
		dumpNode(n.Property, indent+"  ")
		fmt.Fprintf(os.Stderr, "\n%s}", indent)

	case *AssignmentExpression:
		fmt.Fprintf(os.Stderr, "AssignmentExpression {\n")
		fmt.Fprintf(os.Stderr, "%s  operator: %q", indent, n.Operator)
		fmt.Fprintf(os.Stderr, ",\n%s  left: ", indent)
		dumpNode(n.Left, indent+"  ")
		fmt.Fprintf(os.Stderr, ",\n%s  value: ", indent)
		dumpNode(n.Value, indent+"  ")
		fmt.Fprintf(os.Stderr, "\n%s}", indent)

	case *Identifier:
		fmt.Fprintf(os.Stderr, "Identifier { name: %q }", n.Value)

	case *StringLiteral:
		fmt.Fprintf(os.Stderr, "StringLiteral { value: %q }", n.Value)

	case *NumberLiteral:
		fmt.Fprintf(os.Stderr, "NumberLiteral { value: %g }", n.Value)

	case *BooleanLiteral:
		fmt.Fprintf(os.Stderr, "BooleanLiteral { value: %t }", n.Value)

	case *NullLiteral:
		fmt.Fprintf(os.Stderr, "NullLiteral")

	case *UndefinedLiteral:
		fmt.Fprintf(os.Stderr, "UndefinedLiteral")

	case *ThisExpression:
		fmt.Fprintf(os.Stderr, "ThisExpression")

	case *FunctionLiteral:
		fmt.Fprintf(os.Stderr, "FunctionLiteral {\n")
		fmt.Fprintf(os.Stderr, "%s  name: ", indent)
		if n.Name != nil {
			dumpNode(n.Name, indent+"  ")
		} else {
			fmt.Fprintf(os.Stderr, "null")
		}
		fmt.Fprintf(os.Stderr, ",\n%s  typeParameters: [", indent)
		for i, typeParam := range n.TypeParameters {
			if i > 0 {
				fmt.Fprintf(os.Stderr, ", ")
			}
			dumpNode(typeParam, indent+"  ")
		}
		fmt.Fprintf(os.Stderr, "],\n%s  parameters: [", indent)
		for i, param := range n.Parameters {
			if i > 0 {
				fmt.Fprintf(os.Stderr, ", ")
			}
			dumpNode(param, indent+"  ")
		}
		fmt.Fprintf(os.Stderr, "],\n%s  restParameter: ", indent)
		if n.RestParameter != nil {
			dumpNode(n.RestParameter, indent+"  ")
		} else {
			fmt.Fprintf(os.Stderr, "null")
		}
		fmt.Fprintf(os.Stderr, ",\n%s  returnTypeAnnotation: ", indent)
		if n.ReturnTypeAnnotation != nil {
			dumpNode(n.ReturnTypeAnnotation, indent+"  ")
		} else {
			fmt.Fprintf(os.Stderr, "null")
		}
		fmt.Fprintf(os.Stderr, ",\n%s  body: ", indent)
		if n.Body != nil {
			dumpNode(n.Body, indent+"  ")
		} else {
			fmt.Fprintf(os.Stderr, "null")
		}
		fmt.Fprintf(os.Stderr, "\n%s}", indent)

	case *BlockStatement:
		fmt.Fprintf(os.Stderr, "BlockStatement {\n")
		fmt.Fprintf(os.Stderr, "%s  statements: [", indent)
		for i, stmt := range n.Statements {
			if i > 0 {
				fmt.Fprintf(os.Stderr, ", ")
			}
			fmt.Fprintf(os.Stderr, "\n%s    ", indent)
			dumpNode(stmt, indent+"    ")
		}
		if len(n.Statements) > 0 {
			fmt.Fprintf(os.Stderr, "\n%s  ", indent)
		}
		fmt.Fprintf(os.Stderr, "]\n%s}", indent)

	case *Parameter:
		fmt.Fprintf(os.Stderr, "Parameter {\n")
		fmt.Fprintf(os.Stderr, "%s  name: ", indent)
		dumpNode(n.Name, indent+"  ")
		fmt.Fprintf(os.Stderr, ",\n%s  typeAnnotation: ", indent)
		dumpNode(n.TypeAnnotation, indent+"  ")
		fmt.Fprintf(os.Stderr, ",\n%s  optional: %t", indent, n.Optional)
		fmt.Fprintf(os.Stderr, ",\n%s  defaultValue: ", indent)
		dumpNode(n.DefaultValue, indent+"  ")
		fmt.Fprintf(os.Stderr, "\n%s}", indent)

	case *RestParameter:
		fmt.Fprintf(os.Stderr, "RestParameter {\n")
		fmt.Fprintf(os.Stderr, "%s  name: ", indent)
		if n.Name != nil {
			dumpNode(n.Name, indent+"  ")
		} else {
			fmt.Fprintf(os.Stderr, "null")
		}
		fmt.Fprintf(os.Stderr, ",\n%s  typeAnnotation: ", indent)
		if n.TypeAnnotation != nil {
			dumpNode(n.TypeAnnotation, indent+"  ")
		} else {
			fmt.Fprintf(os.Stderr, "null")
		}
		fmt.Fprintf(os.Stderr, "\n%s}", indent)

	default:
		// Fallback for unhandled node types
		fmt.Fprintf(os.Stderr, "%T { /* details not implemented */ }", node)
	}
}
