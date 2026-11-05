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

type jsdocTag struct {
	kind     string // "param", "returns", "type"
	typ      string // raw text between the braces
	name     string // param name
	optional bool   // `[name]` / `[name=x]`
}

var jsdocTagStart = regexp.MustCompile(`^\s*@(param|arg|argument|returns?|type)\b`)

// parseJSDocTags extracts the @param/@returns/@type tags that carry a braced type.
func parseJSDocTags(doc string) []jsdocTag {
	var tags []jsdocTag
	if jsdocOwnsTags(doc) {
		return nil
	}
	for _, line := range strings.Split(doc, "\n") {
		m := jsdocTagStart.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		rest := strings.TrimSpace(line[len(m[0]):])
		if !strings.HasPrefix(rest, "{") {
			continue
		}
		depth, end := 0, -1
		for i := 0; i < len(rest) && end < 0; i++ {
			switch rest[i] {
			case '{':
				depth++
			case '}':
				depth--
				if depth == 0 {
					end = i
				}
			}
		}
		if end < 0 {
			continue
		}
		t := jsdocTag{typ: strings.TrimSpace(rest[1:end])}
		rest = strings.TrimSpace(rest[end+1:])
		switch m[1] {
		case "param", "arg", "argument":
			t.kind = "param"
			if strings.HasPrefix(rest, "[") {
				t.optional = true
				rest = rest[1:]
			}
			i := 0
			for i < len(rest) && (rest[i] == '_' || rest[i] == '$' || rest[i] >= '0' && rest[i] <= '9' || rest[i] >= 'a' && rest[i] <= 'z' || rest[i] >= 'A' && rest[i] <= 'Z' || rest[i] >= 0x80) {
				i++
			}
			if i == 0 || i < len(rest) && rest[i] == '.' {
				continue // nested `name.prop` or no name
			}
			t.name = rest[:i]
		case "returns", "return":
			t.kind = "returns"
		default:
			t.kind = "type"
		}
		tags = append(tags, t)
	}
	return tags
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

// jsdocKnownTypeNames are the capitalized type names resolved without any
// declaration in the file. Other names (a @typedef, @callback, @enum, class or
// import of the file) are not modelled yet, so a tag naming one is left
// unapplied instead of reporting a false TS2304.
var jsdocKnownTypeNames = map[string]bool{
	"Array": true, "ReadonlyArray": true, "Promise": true, "Date": true, "RegExp": true, "Error": true,
	"Map": true, "Set": true, "WeakMap": true, "WeakSet": true, "Function": true, "Record": true,
	"Partial": true, "Required": true, "Readonly": true, "Pick": true, "Omit": true, "Symbol": true,
	"BigInt": true, "Uint8Array": true, "ArrayBuffer": true,
}

var jsdocIdent = regexp.MustCompile(`[A-Za-z_$][\w$]*`)

var jsdocLowerOK = map[string]bool{
	"string": true, "number": true, "boolean": true, "any": true, "object": true, "null": true,
	"undefined": true, "void": true, "never": true, "unknown": true, "bigint": true, "symbol": true,
	"true": true, "false": true, "typeof": true, "keyof": true, "readonly": true, "extends": true,
	"infer": true, "unique": true, "is": true, "new": true,
}

func jsdocNamesResolvable(s string) bool {
	for _, loc := range jsdocIdent.FindAllStringIndex(s, -1) {
		id := s[loc[0]:loc[1]]
		if id[0] >= 'A' && id[0] <= 'Z' {
			if !jsdocKnownTypeNames[id] {
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

// parseJSDocType parses a JSDoc type; nil when it does not parse cleanly.
func parseJSDocType(s string) Expression {
	if !jsdocNamesResolvable(s) {
		return nil
	}
	if strings.TrimSpace(s) == "" || strings.Contains(s, "function(") || strings.Contains(s, "import(") || strings.Contains(s, "typeof ") {
		return nil
	}
	sub := NewParser(lexer.NewLexer(s))
	sub.jsdocSub = true
	t := sub.parseTypeExpression()
	if t == nil || len(sub.errors) > 0 || !sub.peekTokenIs(lexer.EOF) {
		return nil
	}
	return t
}

// applyJSDocToFunction annotates the parameters and return type of a function
// from the `@param` and `@returns` tags of its doc comment.
func applyJSDocToFunction(doc string, params []*Parameter, ret *Expression) {
	for _, t := range parseJSDocTags(doc) {
		switch t.kind {
		case "param":
			for _, prm := range params {
				if prm.Name == nil || prm.Name.Value != t.name || prm.TypeAnnotation != nil || prm.IsThis {
					continue
				}
				src, opt, rest := jsdocTypeToTS(t.typ)
				if rest {
					break
				}
				if ty := parseJSDocType(src); ty != nil {
					prm.TypeAnnotation = ty
					if opt || t.optional {
						prm.Optional = true
					}
				}
				break
			}
		case "returns":
			if *ret == nil {
				src, _, _ := jsdocTypeToTS(t.typ)
				if ty := parseJSDocType(src); ty != nil {
					*ret = ty
				}
			}
		}
	}
}

// applyJSDocToStatement is the JS-file hook run on every documented statement.
func applyJSDocToStatement(stmt Statement, doc string) {
	switch n := stmt.(type) {
	case *ExportNamedDeclaration:
		if n != nil && n.Declaration != nil {
			applyJSDocToStatement(n.Declaration, doc)
		}
	case *ExpressionStatement:
		if n != nil {
			if fl, ok := n.Expression.(*FunctionLiteral); ok && fl != nil {
				applyJSDocToFunction(doc, fl.Parameters, &fl.ReturnTypeAnnotation)
			}
		}
	case *VarStatement:
		if n != nil && len(n.Declarations) == 1 {
			applyJSDocToVarDeclarator(doc, n.Declarations[0])
		}
	case *LetStatement:
		if n != nil && len(n.Declarations) == 1 {
			applyJSDocToVarDeclarator(doc, n.Declarations[0])
		}
	case *ConstStatement:
		if n != nil && len(n.Declarations) == 1 {
			applyJSDocToVarDeclarator(doc, n.Declarations[0])
		}
	}
}

// applyJSDocToVarDeclarator types `/** @type {T} */ var x = ...` and the
// parameters of a function or arrow assigned to the variable.
func applyJSDocToVarDeclarator(doc string, d *VarDeclarator) {
	if d == nil {
		return
	}
	switch v := d.Value.(type) {
	case *FunctionLiteral:
		if v != nil {
			applyJSDocToFunction(doc, v.Parameters, &v.ReturnTypeAnnotation)
		}
	case *ArrowFunctionLiteral:
		if v != nil {
			applyJSDocToFunction(doc, v.Parameters, &v.ReturnTypeAnnotation)
		}
	}
	if d.TypeAnnotation != nil {
		return
	}
	for _, t := range parseJSDocTags(doc) {
		if t.kind != "type" {
			continue
		}
		src, _, _ := jsdocTypeToTS(t.typ)
		if ty := parseJSDocType(src); ty != nil {
			d.TypeAnnotation = ty
		}
		return
	}
}

// IsJavaScriptName reports whether a source name is a JavaScript file.
func IsJavaScriptName(name string) bool { return isJSFileName(name) }
