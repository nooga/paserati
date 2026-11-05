package parser

import (
	"fmt"
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
	jsdocArrayDot = regexp.MustCompile(`\b(Array|Promise|Set|Map)\.<`)
	jsdocBareObj  = regexp.MustCompile(`(^|[^.\w$'"])Object\b([^<]|$)`)
	jsdocWrapper  = regexp.MustCompile(`(^|[^.\w$'"])(String|Number|Boolean)\b`)
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
	s = jsdocClosureFns(s)
	s = jsdocArrayDot.ReplaceAllString(s, "$1<")
	s = jsdocObjectGenerics(s)
	s = jsdocBareObj.ReplaceAllString(s, "${1}any$2")
	s = jsdocWrapper.ReplaceAllStringFunc(s, func(m string) string {
		i := strings.IndexAny(m, "SNB")
		return m[:i] + strings.ToLower(m[i:])
	})
	return s, optional, rest
}

// jsdocClosureFns rewrites Closure `function(A, B=, ...C): R` types into
// `(p1: A, p2?: B, ...p3: C[]) => R`. A `new:` signature, or text it cannot
// split, is left untouched (and so stays unparsed).
func jsdocClosureFns(s string) string {
	var out strings.Builder
	for i := 0; i < len(s); {
		if !strings.HasPrefix(s[i:], "function(") || i > 0 && isIdentByte(s[i-1]) {
			out.WriteByte(s[i])
			i++
			continue
		}
		open := i + len("function")
		end := matchParen(s, open)
		if end < 0 {
			out.WriteString(s[i:])
			return out.String()
		}
		args := splitTopLevel(s[open+1 : end])
		var ps []string
		ok := true
		for n, a := range args {
			a = strings.TrimSpace(a)
			switch {
			case a == "":
			case strings.HasPrefix(a, "new:"):
				ok = false
			case strings.HasPrefix(a, "this:"):
				ps = append(ps, "this: "+jsdocClosureFns(strings.TrimSpace(a[5:])))
			case strings.HasPrefix(a, "..."):
				ps = append(ps, fmt.Sprintf("...p%d: (%s)[]", n+1, jsdocClosureFns(strings.TrimSpace(a[3:]))))
			case strings.HasSuffix(a, "="):
				ps = append(ps, fmt.Sprintf("p%d?: %s", n+1, jsdocClosureFns(strings.TrimSpace(strings.TrimSuffix(a, "=")))))
			default:
				ps = append(ps, fmt.Sprintf("p%d: %s", n+1, jsdocClosureFns(a)))
			}
		}
		if !ok {
			out.WriteString(s[i : end+1])
			i = end + 1
			continue
		}
		j := end + 1
		ret := "any"
		if rest := strings.TrimLeft(s[j:], " "); strings.HasPrefix(rest, ":") {
			k := len(s) - len(rest) + 1
			e := k
			depth := 0
		scan:
			for ; e < len(s); e++ {
				switch s[e] {
				case '(', '<', '[', '{':
					depth++
				case ')', '>', ']', '}':
					if depth == 0 {
						break scan
					}
					depth--
				case ',':
					if depth == 0 {
						break scan
					}
				}
			}
			ret = jsdocClosureFns(strings.TrimSpace(s[k:e]))
			j = e
		}
		fn := "(" + strings.Join(ps, ", ") + ") => " + ret
		if i > 0 || j < len(s) {
			fn = "(" + fn + ")"
		}
		out.WriteString(fn)
		i = j
	}
	return out.String()
}

// jsdocObjectGenerics rewrites `Object<K, V>` / `Object.<K, V>` into the index
// signature type `{ [x: K]: V }`, nested forms and `=>` included.
func jsdocObjectGenerics(s string) string {
	var out strings.Builder
	for i := 0; i < len(s); {
		j := i + len("Object")
		if !strings.HasPrefix(s[i:], "Object") || i > 0 && (isIdentByte(s[i-1]) || s[i-1] == '.') {
			out.WriteByte(s[i])
			i++
			continue
		}
		if j < len(s) && s[j] == '.' {
			j++
		}
		if j >= len(s) || s[j] != '<' {
			out.WriteByte(s[i])
			i++
			continue
		}
		depth, end := 0, -1
		for k := j; k < len(s) && end < 0; k++ {
			switch s[k] {
			case '<', '(', '[', '{':
				depth++
			case '>':
				if s[k-1] == '=' {
					continue
				}
				depth--
				if depth == 0 {
					end = k
				}
			case ')', ']', '}':
				depth--
			}
		}
		if end < 0 {
			out.WriteString(s[i:])
			return out.String()
		}
		args := splitTopLevel(strings.ReplaceAll(s[j+1:end], "=>", "=\x00"))
		if len(args) != 2 {
			out.WriteString(s[i : end+1])
			i = end + 1
			continue
		}
		k := strings.ReplaceAll(strings.TrimSpace(args[0]), "=\x00", "=>")
		v := strings.ReplaceAll(strings.TrimSpace(args[1]), "=\x00", "=>")
		out.WriteString("{ [x: " + jsdocObjectGenerics(k) + "]: " + jsdocObjectGenerics(v) + " }")
		i = end + 1
	}
	return out.String()
}

func isIdentByte(c byte) bool {
	return c == '_' || c == '$' || c >= '0' && c <= '9' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z'
}

// matchParen returns the index of the parenthesis closing the one at open.
func matchParen(s string, open int) int {
	depth := 0
	for i := open; i < len(s); i++ {
		switch s[i] {
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}

// splitTopLevel splits on the commas outside any bracket pair.
func splitTopLevel(s string) []string {
	var parts []string
	depth, start := 0, 0
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '(', '<', '[', '{':
			depth++
		case ')', '>', ']', '}':
			depth--
		case ',':
			if depth == 0 {
				parts = append(parts, s[start:i])
				start = i + 1
			}
		}
	}
	return append(parts, s[start:])
}

// jsdocStringLit matches a string literal type, whose words name no type.
var jsdocStringLit = regexp.MustCompile(`"(?:[^"\\]|\\.)*"|'(?:[^'\\]|\\.)*'`)

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
	s = jsdocStringLit.ReplaceAllStringFunc(s, func(m string) string { return strings.Repeat(" ", len(m)) })
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
// A comment that declares a @callback or @enum owns its tags; one that
// declares a @typedef keeps only the tags that do not belong to the typedef
// (the @param of the function that follows it, say).
func (p *Parser) docTags(rng [2]int) []jsdocRawTag {
	if rng[1] <= rng[0] {
		return nil
	}
	raw := p.l.Input()[rng[0]:rng[1]]
	if strings.Contains(raw, "@callback") || strings.Contains(raw, "@enum") {
		return nil
	}
	tags := scanDocTags(p.l.Input(), rng[0], rng[1])
	if !strings.Contains(raw, "@typedef") {
		return tags
	}
	kept := tags[:0]
	for _, t := range tags {
		switch t.name {
		case "typedef", "property", "prop", "template", "type":
		default:
			kept = append(kept, t)
		}
	}
	return kept
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

// leadingTypeParams names the type parameters of a `<T, U extends X>(...)`
// signature type.
func leadingTypeParams(s string) map[string]bool {
	s = strings.TrimSpace(s)
	if !strings.HasPrefix(s, "<") {
		return nil
	}
	names := map[string]bool{}
	depth := 0
	add := func(rest string) {
		rest = strings.TrimLeft(rest, " ")
		if m := jsdocIdent.FindString(rest); m != "" && strings.HasPrefix(rest, m) {
			names[m] = true
		}
	}
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '<', '(', '[', '{':
			depth++
			if depth == 1 {
				add(s[i+1:])
			}
		case '>', ')', ']', '}':
			depth--
			if depth == 0 {
				return names
			}
		case ',':
			if depth == 1 {
				add(s[i+1:])
			}
		}
	}
	return names
}

// applyJSDocTypeSignature gives a documented function declaration the
// signature of its `@type {(a: A) => R}` tag like tsc's type-tag signature:
// the parameters it leaves unannotated and its return type take the tag's
// types, as do its type parameters and `this`. Explicit `@param`/`@returns`
// tags win, handled before. A tag naming a type alias is not resolved here.
// FIXME: `@type {Alias}` on a function declaration needs the checker.
func (p *Parser) applyJSDocTypeSignature(rng [2]int, fl *FunctionLiteral) {
	for _, t := range p.docTags(rng) {
		if t.name != "type" || t.typOff < 0 {
			continue
		}
		src, _, _ := jsdocTypeToTS(t.typ)
		parsed := p.parseJSDocTypeIn(src, docPad(p.l.Input(), t.typOff), leadingTypeParams(src))
		if obj, isObj := parsed.(*ObjectTypeExpression); isObj && obj != nil && len(obj.Properties) == 1 && obj.Properties[0].IsCallSignature {
			parsed = obj.Properties[0].Type // `{(s: string): void}`
		}
		ft, ok := parsed.(*FunctionTypeExpression)
		if !ok || ft == nil {
			return
		}
		if len(fl.TypeParameters) == 0 {
			fl.TypeParameters = ft.TypeParameters
		}
		hasThis := len(fl.Parameters) > 0 && fl.Parameters[0].IsThis
		if ft.ThisType != nil && !hasThis {
			thisTok := &lexer.Token{Type: lexer.THIS, Literal: "this"}
			fl.Parameters = append([]*Parameter{{Token: thisTok, IsThis: true, TypeAnnotation: ft.ThisType}}, fl.Parameters...)
		}
		i := 0
		for _, prm := range fl.Parameters {
			if prm.IsThis {
				continue
			}
			if i < len(ft.Parameters) && prm.TypeAnnotation == nil && prm.Pattern == nil {
				prm.TypeAnnotation = ft.Parameters[i]
				if i < len(ft.OptionalParams) && ft.OptionalParams[i] {
					prm.Optional = true
				}
			}
			i++
		}
		if fl.RestParameter != nil && fl.RestParameter.TypeAnnotation == nil && ft.RestParameter != nil && i >= len(ft.Parameters) {
			fl.RestParameter.TypeAnnotation = ft.RestParameter
		}
		if fl.ReturnTypeAnnotation == nil {
			fl.ReturnTypeAnnotation = ft.ReturnType
		}
		return
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
				p.applyJSDocTypeSignature(rng, fl)
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
