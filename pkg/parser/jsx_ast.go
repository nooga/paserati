package parser

import (
	"bytes"

	"github.com/nooga/paserati/pkg/lexer"
)

// JSXElement is a JSX element (`<Tag a="x">children</Tag>`, `<Tag />`) or,
// with a nil TagName, a fragment (`<>children</>`).
type JSXElement struct {
	BaseExpression
	Token *lexer.Token // the opening '<'
	// TagName is an Identifier (`div`, `Foo`, `data-x`), a MemberExpression
	// (`a.b.C`), a ThisExpression or a JSXNamespacedName (`svg:rect`); nil for
	// a fragment.
	TagName       Expression
	TypeArguments []Expression
	Attributes    []*JSXAttribute
	Children      []Expression // JSXText, JSXExpressionContainer, JSXElement
	SelfClosing   bool
	// ClosingName is the closing tag's name (nil when self-closing or a
	// fragment); ClosingToken is the closing tag's '<'.
	ClosingName  Expression
	ClosingToken *lexer.Token
	// EndPos is the byte offset just past the element.
	EndPos int

	// Checked is the call the checker checks the element as (see
	// checker/jsx.go); nil until then.
	Checked Expression
}

func (e *JSXElement) expressionNode()      {}
func (e *JSXElement) TokenLiteral() string { return e.Token.Literal }
func (e *JSXElement) IsFragment() bool     { return e.TagName == nil }
func (e *JSXElement) String() string {
	var out bytes.Buffer
	out.WriteString("<")
	if e.TagName != nil {
		out.WriteString(e.TagName.String())
	}
	for _, a := range e.Attributes {
		out.WriteString(" ")
		out.WriteString(a.String())
	}
	if e.SelfClosing {
		out.WriteString(" />")
		return out.String()
	}
	out.WriteString(">")
	for _, ch := range e.Children {
		out.WriteString(ch.String())
	}
	out.WriteString("</")
	if e.TagName != nil {
		out.WriteString(e.TagName.String())
	}
	out.WriteString(">")
	return out.String()
}

// JSXAttribute is `name`, `name="v"`, `name={expr}`, `name=<el/>`, or with a
// non-nil Spread, `{...expr}`.
type JSXAttribute struct {
	Token *lexer.Token // the name, or the '{' of a spread
	Name  string       // may contain '-' or a namespace prefix (`xlink:href`)
	// Value is a StringLiteral, a JSXExpressionContainer or a JSXElement; nil
	// for a bare name, which is `true`.
	Value  Expression
	Spread Expression
}

func (a *JSXAttribute) String() string {
	if a.Spread != nil {
		return "{..." + a.Spread.String() + "}"
	}
	if a.Value == nil {
		return a.Name
	}
	return a.Name + "=" + a.Value.String()
}

// JSXText is literal text between tags.
type JSXText struct {
	BaseExpression
	Token *lexer.Token
	Value string // the raw text
}

func (t *JSXText) expressionNode()      {}
func (t *JSXText) TokenLiteral() string { return t.Token.Literal }
func (t *JSXText) String() string       { return t.Value }

// JSXExpressionContainer is `{expr}` (an attribute value or a child), `{}` or
// `{/* comment */}` with a nil Expression, or the child `{...expr}`.
type JSXExpressionContainer struct {
	BaseExpression
	Token      *lexer.Token // the '{'
	Expression Expression
	Spread     bool
}

func (c *JSXExpressionContainer) expressionNode()      {}
func (c *JSXExpressionContainer) TokenLiteral() string { return c.Token.Literal }
func (c *JSXExpressionContainer) String() string {
	if c.Expression == nil {
		return "{}"
	}
	if c.Spread {
		return "{..." + c.Expression.String() + "}"
	}
	return "{" + c.Expression.String() + "}"
}

// JSXNamespacedName is a tag or attribute name with a namespace (`svg:rect`).
type JSXNamespacedName struct {
	BaseExpression
	Token     *lexer.Token
	Namespace string
	Name      string
}

func (n *JSXNamespacedName) expressionNode()      {}
func (n *JSXNamespacedName) TokenLiteral() string { return n.Token.Literal }
func (n *JSXNamespacedName) String() string       { return n.Namespace + ":" + n.Name }

// JSXTagType is a checker-made expression whose type is fixed: the callee of
// the call a JSX element is checked as.
type JSXTagType struct {
	BaseExpression
	Token *lexer.Token
}

func (t *JSXTagType) expressionNode()      {}
func (t *JSXTagType) TokenLiteral() string { return t.Token.Literal }
func (t *JSXTagType) String() string       { return "<jsx tag>" }
