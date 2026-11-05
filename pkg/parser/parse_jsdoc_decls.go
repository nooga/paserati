package parser

import (
	"regexp"
	"strings"

	"github.com/nooga/paserati/pkg/lexer"
)

// JSDoc type declarations in JavaScript files: `@typedef`, `@callback`,
// `@import` and `@template`. tsc turns each into a declaration scoped to the
// file (exported from a module); here they become synthesized TypeAlias and
// type-only import statements prepended to the program, so the checker
// resolves the names like any other type.

// jsdocRawTag is one `@tag {type} rest` of a doc comment with the absolute
// source offset of its braced type, so the type parses at its real position.
type jsdocRawTag struct {
	name   string
	typ    string
	typOff int
	rest   string
	tagOff int // offset of the '@'
}

var jsdocTagName = regexp.MustCompile(`^@([A-Za-z]+)`)

// blankDocStars returns raw with each line's leading `*` decoration and the
// comment delimiters replaced by spaces, so offsets stay valid.
func blankDocStars(raw string) []byte {
	b := []byte(raw)
	lineStart := true
	for i := 0; i < len(b); i++ {
		c := b[i]
		if c == '\n' {
			lineStart = true
			continue
		}
		if lineStart {
			if c == ' ' || c == '\t' || c == '\r' {
				continue
			}
			if c == '/' && i+2 < len(b) && b[i+1] == '*' && b[i+2] == '*' {
				b[i], b[i+1], b[i+2] = ' ', ' ', ' '
				i += 2
				continue
			}
			if c == '*' {
				b[i] = ' '
				continue
			}
			lineStart = false
		}
	}
	if n := len(b); n >= 2 && b[n-2] == '*' && b[n-1] == '/' {
		b[n-2], b[n-1] = ' ', ' '
	}
	return b
}

func scanDocTags(src string, start, end int) []jsdocRawTag {
	text := string(blankDocStars(src[start:end]))
	var tags []jsdocRawTag
	pos := 0
	for pos < len(text) {
		nl := strings.IndexByte(text[pos:], '\n')
		lineEnd := len(text)
		if nl >= 0 {
			lineEnd = pos + nl
		}
		line := text[pos:lineEnd]
		trimmed := strings.TrimLeft(line, " \t\r")
		if strings.HasPrefix(trimmed, "@") {
			at := pos + (len(line) - len(trimmed))
			if m := jsdocTagName.FindStringSubmatch(text[at:]); m != nil {
				tag := jsdocRawTag{name: m[1], typOff: -1, tagOff: start + at}
				i := at + len(m[0])
				for i < len(text) && (text[i] == ' ' || text[i] == '\t') {
					i++
				}
				if i < len(text) && text[i] == '{' {
					depth, j := 0, i
					for ; j < len(text); j++ {
						if text[j] == '{' {
							depth++
						} else if text[j] == '}' {
							depth--
							if depth == 0 {
								break
							}
						}
					}
					if j < len(text) {
						tag.typ = strings.TrimSpace(text[i+1 : j])
						tag.typOff = start + i + 1
						i = j + 1
					}
				}
				restEnd := strings.IndexByte(text[i:], '\n')
				if restEnd < 0 {
					restEnd = len(text) - i
				}
				tag.rest = strings.TrimSpace(text[i : i+restEnd])
				if tag.typOff < 0 && (tag.name == "property" || tag.name == "prop" || tag.name == "param") {
					// `@property name {type}`: the type follows the name.
					line := text[i : i+restEnd]
					if open := strings.IndexByte(line, '{'); open > 0 && strings.HasSuffix(strings.TrimSpace(line), "}") && jsdocWord.MatchString(strings.TrimSpace(line)) {
						name := strings.TrimSpace(line[:open])
						tag.typ = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(line[open+1:]), "}"))
						tag.typOff = start + i + open + 1
						tag.rest = name
					}
				}
				tags = append(tags, tag)
			}
		}
		pos = lineEnd + 1
	}
	return tags
}

var jsdocWord = regexp.MustCompile(`^[A-Za-z_$][\w$]*`)

// docPad returns spaces (newlines kept) covering src[:off], so a snippet that
// follows it lexes at its original line and column.
func docPad(src string, off int) string {
	if off > len(src) {
		off = len(src)
	}
	b := []byte(src[:off])
	for i, c := range b {
		if c != '\n' {
			b[i] = ' '
		}
	}
	return string(b)
}

// jsdocTypeAt parses a JSDoc type written at source offset off (-1: no known
// position); unresolvable or unparsable text yields nil.
func (p *Parser) jsdocTypeAt(typ string, off int, locals map[string]bool) Expression {
	src, _, _ := jsdocTypeToTS(typ)
	pad := ""
	if off >= 0 {
		pad = docPad(p.l.Input(), off)
	}
	return p.parseJSDocTypeIn(src, pad, locals)
}

func anyTypeExpr() Expression {
	return &Identifier{Token: &lexer.Token{Type: lexer.IDENT, Literal: "any"}, Value: "any"}
}

func synthIdent(name string) *Identifier {
	return &Identifier{Token: &lexer.Token{Type: lexer.IDENT, Literal: name}, Value: name}
}

// jsdocTemplateParams builds the type parameters of the `@template` tags.
func (p *Parser) jsdocTemplateParams(tags []jsdocRawTag) []*TypeParameter {
	var out []*TypeParameter
	for _, t := range tags {
		if t.name != "template" {
			continue
		}
		var constraint Expression
		if t.typ != "" {
			constraint = p.jsdocTypeAt(t.typ, t.typOff, nil)
		}
		for _, part := range strings.Split(t.rest, ",") {
			part = strings.TrimSpace(part)
			var def Expression
			if strings.HasPrefix(part, "[") {
				inner := strings.TrimSuffix(strings.TrimPrefix(part, "["), "]")
				if eq := strings.Index(inner, "="); eq >= 0 {
					def = p.jsdocTypeAt(strings.TrimSpace(inner[eq+1:]), -1, nil)
					inner = inner[:eq]
				}
				part = strings.TrimSpace(inner)
			}
			name := jsdocWord.FindString(part)
			if name == "" {
				break
			}
			if after := strings.TrimSpace(part[len(name):]); after != "" && !strings.HasPrefix(after, "-") {
				break // a trailing description, not another parameter
			}
			out = append(out, &TypeParameter{Token: &lexer.Token{Type: lexer.IDENT, Literal: name}, Name: synthIdent(name), Constraint: constraint, DefaultType: def})
		}
	}
	return out
}

func tpNames(tps []*TypeParameter) map[string]bool {
	if len(tps) == 0 {
		return nil
	}
	m := map[string]bool{}
	for _, tp := range tps {
		m[tp.Name.Value] = true
	}
	return m
}

// jsdocParamName reads `name`, `[name]` or `[name=default]` of a param tag.
func jsdocParamName(rest string) (name string, optional bool) {
	if strings.HasPrefix(rest, "[") {
		optional = true
		rest = rest[1:]
	}
	name = jsdocWord.FindString(rest)
	if name != "" && len(rest) > len(name) && rest[len(name)] == '.' {
		return "", false // nested `name.prop`
	}
	return name, optional
}

// parseJSDocDeclarations returns the alias and import statements declared by
// the file's `@typedef`, `@callback` and `@import` tags.
func (p *Parser) parseJSDocDeclarations() (imports, aliases []Statement) {
	src := p.l.Input()
	for _, r := range p.l.DocRanges() {
		raw := src[r[0]:r[1]]
		if !strings.Contains(raw, "@typedef") && !strings.Contains(raw, "@callback") && !strings.Contains(raw, "@import") {
			continue
		}
		tags := scanDocTags(src, r[0], r[1])
		if strings.Contains(raw, "@import") {
			imports = append(imports, p.parseJSDocImports(src, r[0], r[1], tags)...)
		}
		tmpl := p.jsdocTemplateParams(tags)
		for i, t := range tags {
			if t.name != "typedef" && t.name != "callback" {
				continue
			}
			end := len(tags)
			for j := i + 1; j < len(tags); j++ {
				if tags[j].name == "typedef" || tags[j].name == "callback" {
					end = j
					break
				}
			}
			members := tags[i+1 : end]
			rest := t.rest
			name := jsdocWord.FindString(rest)
			if name == "" || len(rest) > len(name) && rest[len(name)] == '.' {
				continue
			}
			locals := tpNames(tmpl)
			var ty Expression
			if t.name == "callback" {
				ty = p.jsdocCallbackType(members, locals)
			} else {
				ty = p.jsdocTypedefType(t, members, locals)
			}
			if ty == nil {
				ty = anyTypeExpr()
			}
			aliases = append(aliases, &TypeAliasStatement{
				Token:          &lexer.Token{Type: lexer.TYPE, Literal: "type"},
				Name:           synthIdent(name),
				TypeParameters: tmpl,
				Type:           ty,
			})
		}
	}
	return imports, aliases
}

func (p *Parser) jsdocTypedefType(t jsdocRawTag, members []jsdocRawTag, locals map[string]bool) Expression {
	var props []*ObjectTypeProperty
	for _, m := range members {
		if m.name != "property" && m.name != "prop" {
			continue
		}
		name, opt := jsdocParamName(m.rest)
		if name == "" {
			continue
		}
		pt := p.jsdocTypeAt(m.typ, m.typOff, locals)
		if pt == nil {
			pt = anyTypeExpr()
		}
		if strings.HasSuffix(strings.TrimSpace(m.typ), "=") {
			opt = true
		}
		props = append(props, &ObjectTypeProperty{Name: synthIdent(name), Type: pt, Optional: opt})
	}
	base := strings.TrimSpace(t.typ)
	if len(props) > 0 && (base == "" || base == "Object" || base == "object") {
		return &ObjectTypeExpression{Token: &lexer.Token{Type: lexer.LBRACE, Literal: "{"}, Properties: props}
	}
	if base == "" {
		return nil
	}
	return p.jsdocTypeAt(t.typ, t.typOff, locals)
}

func (p *Parser) jsdocCallbackType(members []jsdocRawTag, locals map[string]bool) Expression {
	fte := &FunctionTypeExpression{Token: &lexer.Token{Type: lexer.LPAREN, Literal: "("}}
	for _, m := range members {
		switch m.name {
		case "param", "arg", "argument":
			name, opt := jsdocParamName(m.rest)
			if name == "" {
				continue
			}
			typ := strings.TrimSpace(m.typ)
			if strings.HasPrefix(typ, "...") {
				elem := p.jsdocTypeAt(strings.TrimPrefix(typ, "..."), m.typOff, locals)
				if elem == nil {
					elem = anyTypeExpr()
				}
				fte.RestParameter = &ArrayTypeExpression{Token: &lexer.Token{Type: lexer.LBRACKET, Literal: "["}, ElementType: elem}
				fte.ParameterNames = append(fte.ParameterNames, name)
				continue
			}
			pt := p.jsdocTypeAt(typ, m.typOff, locals)
			if pt == nil {
				pt = anyTypeExpr()
			}
			if strings.HasSuffix(typ, "=") {
				opt = true
			}
			fte.Parameters = append(fte.Parameters, pt)
			fte.OptionalParams = append(fte.OptionalParams, opt)
			fte.ParameterNames = append(fte.ParameterNames, name)
		case "return", "returns":
			if m.typ != "" {
				fte.ReturnType = p.jsdocTypeAt(m.typ, m.typOff, locals)
			}
		}
	}
	if fte.ReturnType == nil {
		fte.ReturnType = anyTypeExpr()
	}
	return fte
}

var jsdocImportRe = regexp.MustCompile(`(?s)^@import\b(.*?\bfrom\s*(?:"[^"]*"|'[^']*'))`)

// parseJSDocImports turns `@import { A } from "m"` into a type-only import.
func (p *Parser) parseJSDocImports(src string, start, end int, tags []jsdocRawTag) []Statement {
	text := string(blankDocStars(src[start:end]))
	var out []Statement
	for _, t := range tags {
		if t.name != "import" {
			continue
		}
		m := jsdocImportRe.FindStringSubmatch(text[t.tagOff-start:])
		if m == nil {
			continue
		}
		code := "import type " + strings.TrimSpace(m[1]) + ";"
		sub := NewParser(lexer.NewLexer(docPad(src, t.tagOff) + code))
		sub.jsdocSub = true
		prog, errs := sub.ParseProgram()
		if prog == nil || len(errs) > 0 {
			continue
		}
		out = append(out, prog.Statements...)
	}
	return out
}

// prependJSDocDeclarations declares the file's JSDoc typedefs, callbacks and
// imports ahead of its statements; a module exports its typedefs like tsc.
func (p *Parser) prependJSDocDeclarations(program *Program) {
	imports, aliases := p.parseJSDocDeclarations()
	if len(imports) == 0 && len(aliases) == 0 {
		return
	}
	isModule := false
	for _, st := range program.Statements {
		switch st.(type) {
		case *ImportDeclaration, *ExportNamedDeclaration, *ExportDefaultDeclaration, *ExportAllDeclaration:
			isModule = true
		}
	}
	if isModule {
		for i, a := range aliases {
			aliases[i] = &ExportNamedDeclaration{Token: &lexer.Token{Type: lexer.EXPORT, Literal: "export"}, Declaration: a, IsTypeOnly: true}
		}
	}
	decls := append(imports, aliases...)
	program.Statements = append(decls, program.Statements...)
}

// applyJSDocCast reads `/** @type {T} */ (expr)` as the assertion `(expr as T)`
// and `/** @satisfies {T} */ (expr)` as `(expr satisfies T)`.
func (p *Parser) applyJSDocCast(start int, expr Expression) Expression {
	ds, de, ok := p.l.DocRangeBefore(start)
	if !ok {
		return expr
	}
	isFn := false
	switch expr.(type) {
	case *ArrowFunctionLiteral, *FunctionLiteral, *TypeAssertionExpression:
		isFn = true
	}
	for _, t := range p.docTags([2]int{ds, de}) {
		if t.typOff < 0 || t.name != "type" && t.name != "satisfies" || t.name == "type" && isFn {
			continue
		}
		src, _, _ := jsdocTypeToTS(t.typ)
		ty := p.parseJSDocTypeIn(src, docPad(p.l.Input(), t.typOff), nil)
		if ty == nil {
			return expr
		}
		if t.name == "satisfies" {
			return &SatisfiesExpression{Token: &lexer.Token{Type: lexer.SATISFIES, Literal: "satisfies"}, Expression: expr, TargetType: ty}
		}
		return &TypeAssertionExpression{Token: &lexer.Token{Type: lexer.AS, Literal: "as"}, Expression: expr, TargetType: ty}
	}
	return expr
}

// jsdocClassTemplates builds the type parameters of the `@template` tags in
// the doc comment before the class declaration whose `class` token is at
// classPos, looking back over `export`, `default`, `abstract` and `declare`.
func (p *Parser) jsdocClassTemplates(classPos int) []*TypeParameter {
	src := p.l.Input()
	pos := classPos
	for {
		i := pos
		for i > 0 && (src[i-1] == ' ' || src[i-1] == '\t' || src[i-1] == '\n' || src[i-1] == '\r') {
			i--
		}
		moved := false
		for _, kw := range []string{"export", "default", "abstract", "declare"} {
			if strings.HasSuffix(src[:i], kw) && (i-len(kw) == 0 || !isJSIdentByte(src[i-len(kw)-1])) {
				pos = i - len(kw)
				moved = true
				break
			}
		}
		if !moved {
			break
		}
	}
	ds, de, ok := p.l.DocRangeBefore(pos)
	if !ok {
		return nil
	}
	return p.jsdocTemplateParams(p.docTags([2]int{ds, de}))
}

func isJSIdentByte(c byte) bool {
	return c == '_' || c == '$' || c >= '0' && c <= '9' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= 0x80
}
