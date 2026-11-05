package parser

import (
	"regexp"
	"strings"

	"github.com/nooga/paserati/pkg/lexer"
)

// JSDoc types in JavaScript files. tsc reads `@param {T} name`, `@returns {T}`
// and `@type {T}` of a declaration as its type annotation (JS files only); the
// tag type is parsed here with the TypeScript type parser after rewriting the
// JSDoc-only spellings (`*`, `?T`, `Array.<T>`, `T=`, `Object`, ...).

func isJSFileName(name string) bool {
	n := strings.ToLower(name)
	for _, ext := range []string{".js", ".jsx", ".mjs", ".cjs"} {
		if strings.HasSuffix(n, ext) {
			return true
		}
	}
	return false
}

var (
	jsdocArrayDot  = regexp.MustCompile(`\b(Array|Promise|Set|Map)\.<`)
	jsdocObjectGen = regexp.MustCompile(`\bObject\.?<\s*([^,<>]+?)\s*,\s*([^<>]+?)\s*>`)
	jsdocBareObj   = regexp.MustCompile(`(^|[^.\w$'"])Object\b([^<]|$)`)
	jsdocWrapper   = regexp.MustCompile(`(^|[^.\w$'"])(String|Number|Boolean)\b`)
)

// jsdocTypeToTS rewrites a JSDoc type into TypeScript syntax, reporting
// whether the type is `T=` (optional) or `...T` (rest).
func jsdocTypeToTS(s string) (out string, optional, rest bool) {
	s = strings.TrimSpace(s)
	if strings.HasSuffix(s, "=") {
		optional = true
		s = strings.TrimSpace(strings.TrimSuffix(s, "="))
	}
	if strings.HasPrefix(s, "...") {
		rest = true
		s = strings.TrimSpace(s[3:])
	}
	if s == "*" || s == "?" {
		return "any", optional, rest
	}
	s = strings.TrimPrefix(s, "!")
	if strings.HasPrefix(s, "?") {
		s = "(" + s[1:] + ") | null"
	}
	s = jsdocArrayDot.ReplaceAllString(s, "$1<")
	s = jsdocObjectGen.ReplaceAllString(s, "{ [x: $1]: $2 }")
	s = jsdocBareObj.ReplaceAllString(s, "${1}any$2")
	s = jsdocWrapper.ReplaceAllStringFunc(s, func(m string) string {
		i := strings.IndexAny(m, "SNB")
		return m[:i] + strings.ToLower(m[i:])
	})
	return s, optional, rest
}

var jsdocIdent = regexp.MustCompile(`[A-Za-z_$][\w$]*`)

var jsdocLowerOK = map[string]bool{
	"string": true, "number": true, "boolean": true, "any": true, "object": true, "null": true,
	"undefined": true, "void": true, "never": true, "unknown": true, "bigint": true, "symbol": true,
	"true": true, "false": true, "typeof": true, "keyof": true, "readonly": true, "extends": true,
	"infer": true, "unique": true, "is": true, "new": true,
}

// jsdocKnownTypeNames are the global type names resolved without any
// declaration in the file. A name that is neither one of these nor declared
// in the file (a require()d class, a @enum, a constructor function, another
// lib type) is not modelled yet, so a tag naming it is left unapplied or
// annotated any instead of reporting a false TS2304.
var jsdocKnownTypeNames = map[string]bool{
	"Array": true, "ReadonlyArray": true, "Promise": true, "Date": true, "RegExp": true, "Error": true,
	"Map": true, "Set": true, "WeakMap": true, "WeakSet": true, "Function": true, "Record": true,
	"Partial": true, "Required": true, "Readonly": true, "Pick": true, "Omit": true, "Symbol": true,
	"BigInt": true, "Uint8Array": true, "ArrayBuffer": true,
}

var jsdocDeclRe = regexp.MustCompile(`(?m)(?:^|[\s;}])(?:class|interface)\s+([A-Za-z_$][\w$]*)|@import\b[^\n]*`)

// declaredTypeNames are the type names the file itself declares: its classes,
// interfaces, and JSDoc typedefs, callbacks and imports.
func (p *Parser) declaredTypeNames() map[string]bool {
	if p.jsdocDeclared == nil {
		p.jsdocDeclared = map[string]bool{}
		src := p.l.Input()
		for _, m := range jsdocDeclRe.FindAllStringSubmatch(src, -1) {
			switch {
			case m[1] != "":
				p.jsdocDeclared[m[1]] = true
			default:
				for _, id := range jsdocIdent.FindAllString(strings.TrimPrefix(m[0], "@import"), -1) {
					p.jsdocDeclared[id] = true
				}
			}
		}
		for _, line := range jsdocTypedefLine.FindAllStringSubmatch(src, -1) {
			p.jsdocDeclared[line[1]] = true
		}
	}
	return p.jsdocDeclared
}

var jsdocTypedefLine = regexp.MustCompile(`@(?:typedef|callback)\b(?:[ \t]*\{[^\n]*\})?[ \t]*([A-Za-z_$][\w$]*)`)

var jsdocTemplateLine = regexp.MustCompile(`@template\b[^\n]*`)

// fileTemplateNames are the names the file's `@template` tags declare. A tag
// that mentions one outside the declaration that owns it (a class method using
// the class's parameter) is not modelled yet and stays unapplied.
func (p *Parser) fileTemplateNames() map[string]bool {
	if p.jsdocTemplates == nil {
		p.jsdocTemplates = map[string]bool{}
		for _, line := range jsdocTemplateLine.FindAllString(p.l.Input(), -1) {
			for _, id := range jsdocIdent.FindAllString(strings.TrimPrefix(line, "@template"), -1) {
				p.jsdocTemplates[id] = true
			}
		}
	}
	return p.jsdocTemplates
}

func (p *Parser) jsdocNamesResolvable(s string, locals map[string]bool) bool {
	tmpl := p.fileTemplateNames()
	for _, loc := range jsdocIdent.FindAllStringIndex(s, -1) {
		id := s[loc[0]:loc[1]]
		if locals[id] {
			continue
		}
		if id[0] >= 'A' && id[0] <= 'Z' {
			if tmpl[id] || !jsdocKnownTypeNames[id] && !p.declaredTypeNames()[id] {
				return false
			}
			continue
		}
		if jsdocLowerOK[id] {
			continue
		}
		// a lowercase name is fine as a property key (`{ a: T }`, `{ a?: T }`)
		rest := strings.TrimLeft(s[loc[1]:], " ")
		if strings.HasPrefix(rest, ":") || strings.HasPrefix(rest, "?:") {
			continue
		}
		return false
	}
	return true
}

// jsdocOwnsTags reports a doc comment that belongs to a @typedef, @callback or
// @enum: its @param/@type tags describe that type, not the next declaration.
func jsdocOwnsTags(doc string) bool {
	return strings.Contains(doc, "@typedef") || strings.Contains(doc, "@callback") || strings.Contains(doc, "@enum")
}

// parseJSDocTypeIn parses a JSDoc type already rewritten to TypeScript syntax,
// preceded by pad (see docPad) so its tokens carry their source positions; nil
// when it does not parse cleanly.
func (p *Parser) parseJSDocTypeIn(s, pad string, locals map[string]bool) Expression {
	if !p.jsdocNamesResolvable(s, locals) {
		return nil
	}
	if strings.TrimSpace(s) == "" || strings.Contains(s, "function(") || strings.Contains(s, "import(") || strings.Contains(s, "typeof ") {
		return nil
	}
	sub := NewParser(lexer.NewLexer(pad + s))
	sub.jsdocSub = true
	sub.jsdocTemplates = p.fileTemplateNames()
	sub.jsdocDeclared = p.declaredTypeNames()
	t := sub.parseTypeExpression()
	if t == nil || len(sub.errors) > 0 || !sub.peekTokenIs(lexer.EOF) {
		return nil
	}
	return t
}

// docTags scans the tags of the doc comment ending at the given source range.
// A comment that declares a @typedef, @callback or @enum owns its tags.
func (p *Parser) docTags(rng [2]int) []jsdocRawTag {
	if rng[1] <= rng[0] || jsdocOwnsTags(p.l.Input()[rng[0]:rng[1]]) {
		return nil
	}
	return scanDocTags(p.l.Input(), rng[0], rng[1])
}

// applyJSDocToFunction annotates the parameters and return type of a function
// from the `@param` and `@returns` tags of its doc comment, and gives it the
// type parameters of its `@template` tags.
func (p *Parser) applyJSDocToFunction(rng [2]int, params *[]*Parameter, ret *Expression, tps *[]*TypeParameter) {
	tags := p.docTags(rng)
	if len(tags) == 0 {
		return
	}
	locals := map[string]bool{}
	for n := range p.classTplLocals {
		locals[n] = true
	}
	if len(*tps) == 0 {
		if tp := p.jsdocTemplateParams(tags); len(tp) > 0 {
			*tps = tp
		}
	}
	for n := range tpNames(*tps) {
		locals[n] = true
	}
	for _, t := range tags {
		switch t.name {
		case "param", "arg", "argument":
			if t.typOff < 0 {
				continue
			}
			name, optTag := jsdocParamName(t.rest)
			if name == "" {
				continue
			}
			for _, prm := range *params {
				if prm.Name == nil || prm.Name.Value != name || prm.TypeAnnotation != nil || prm.IsThis {
					continue
				}
				src, opt, rest := jsdocTypeToTS(t.typ)
				if rest {
					break
				}
				ty := p.parseJSDocTypeIn(src, docPad(p.l.Input(), t.typOff), locals)
				if ty == nil && strings.TrimSpace(src) != "" {
					// A type the parser cannot express (a `function(...)` type,
					// a template parameter of an enclosing declaration) still
					// makes the parameter annotated: any, not an implicit any.
					ty = anyTypeExpr()
				}
				if ty != nil {
					prm.TypeAnnotation = ty
					if opt || optTag {
						prm.Optional = true
					}
				}
				break
			}
		case "this":
			if t.typOff < 0 || len(*params) > 0 && (*params)[0].IsThis {
				continue
			}
			src, _, _ := jsdocTypeToTS(t.typ)
			if ty := p.parseJSDocTypeIn(src, docPad(p.l.Input(), t.typOff), locals); ty != nil {
				thisTok := &lexer.Token{Type: lexer.THIS, Literal: "this"}
				*params = append([]*Parameter{{Token: thisTok, IsThis: true, TypeAnnotation: ty}}, *params...)
			}
		case "returns", "return":
			if *ret == nil && t.typOff >= 0 {
				src, _, _ := jsdocTypeToTS(t.typ)
				if ty := p.parseJSDocTypeIn(src, docPad(p.l.Input(), t.typOff), locals); ty != nil {
					*ret = ty
				}
			}
		}
	}
}

// applyJSDocToStatement is the JS-file hook run on every documented statement.
func (p *Parser) applyJSDocToStatement(stmt Statement, rng [2]int) {
	switch n := stmt.(type) {
	case *ExportNamedDeclaration:
		if n != nil && n.Declaration != nil {
			p.applyJSDocToStatement(n.Declaration, rng)
		}
	case *ExpressionStatement:
		if n != nil {
			if fl, ok := n.Expression.(*FunctionLiteral); ok && fl != nil {
				p.applyJSDocToFunction(rng, &fl.Parameters, &fl.ReturnTypeAnnotation, &fl.TypeParameters)
			}
		}
	case *VarStatement:
		if n != nil && len(n.Declarations) == 1 {
			p.applyJSDocToVarDeclarator(rng, n.Declarations[0])
		}
	case *LetStatement:
		if n != nil && len(n.Declarations) == 1 {
			p.applyJSDocToVarDeclarator(rng, n.Declarations[0])
		}
	case *ConstStatement:
		if n != nil && len(n.Declarations) == 1 {
			p.applyJSDocToVarDeclarator(rng, n.Declarations[0])
		}
	}
}

// applyJSDocToVarDeclarator types `/** @type {T} */ var x = ...` and the
// parameters of a function or arrow assigned to the variable.
func (p *Parser) applyJSDocToVarDeclarator(rng [2]int, d *VarDeclarator) {
	if d == nil {
		return
	}
	switch v := d.Value.(type) {
	case *FunctionLiteral:
		if v != nil {
			p.applyJSDocToFunction(rng, &v.Parameters, &v.ReturnTypeAnnotation, &v.TypeParameters)
		}
	case *ArrowFunctionLiteral:
		if v != nil {
			p.applyJSDocToFunction(rng, &v.Parameters, &v.ReturnTypeAnnotation, &v.TypeParameters)
		}
	}
	if d.TypeAnnotation != nil {
		return
	}
	for _, t := range p.docTags(rng) {
		if t.name != "type" || t.typOff < 0 {
			continue
		}
		src, _, _ := jsdocTypeToTS(t.typ)
		if ty := p.parseJSDocTypeIn(src, docPad(p.l.Input(), t.typOff), nil); ty != nil {
			d.TypeAnnotation = ty
		}
		return
	}
}

// IsJavaScriptName reports whether a source name is a JavaScript file.
func IsJavaScriptName(name string) bool { return isJSFileName(name) }
