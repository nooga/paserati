package checker

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/nooga/paserati/pkg/modules"
	"github.com/nooga/paserati/pkg/nodemodules"
	"github.com/nooga/paserati/pkg/parser"
	"github.com/nooga/paserati/pkg/types"
)

// With --importHelpers, the helpers that downleveled syntax needs come from
// the "tslib" module, and tsc checks that the module exports each helper the
// file's syntax needs (checkExternalEmitHelpers): TS2343 for a missing one
// and TS2354 when tslib cannot be found. Each helper is reported once, at
// the first node that needs it.
//
// FIXME: only the helpers of ECMAScript and legacy decorators and of the
// module interop (__importDefault, __importStar, __exportStar) are checked;
// the others (__awaiter, __generator, __rest, __spreadArray, __assign,
// __extends, __values, __read, __classPrivateField*, __makeTemplateObject,
// __addDisposableResource, __metadata, __param, ...) and the TS2807
// parameter-count checks of __spreadArray and __classPrivateFieldGet/Set are
// not. Decorated constructor parameters do not count as decorating a class.

// SetImportHelpers mirrors --importHelpers.
func (c *Checker) SetImportHelpers(on bool) { c.importHelpers = on }

// SetESModuleInteropOff records an explicit --esModuleInterop false (the
// option defaults to true).
func (c *Checker) SetESModuleInteropOff(off bool) { c.esModuleInteropOff = off }

// emitHelperState is what the check of one file's helpers remembers.
type emitHelperState struct {
	resolved bool
	found    bool
	exports  map[string]types.Type
	reported map[string]bool
}

// requireHelpers reports the helpers names that tslib does not export, at
// node.
func (c *Checker) requireHelpers(node parser.Node, names ...string) {
	st := c.helperState
	if st == nil {
		return
	}
	if !st.resolved {
		st.resolved = true
		if c.moduleLoader != nil {
			if loaded, err := c.moduleLoader.LoadModule("tslib", c.currentFilePath()); err == nil {
				if rec, ok := loaded.(*modules.ModuleRecord); ok && rec != nil {
					st.exports, st.found = rec.Exports, true
				}
			}
		}
		if !st.found {
			c.addErrorWithCode(node, "TS2354", "This syntax requires an imported helper but module 'tslib' cannot be found.")
		}
	}
	if !st.found {
		return
	}
	for _, name := range names {
		if st.reported[name] {
			continue
		}
		st.reported[name] = true
		if _, ok := st.exports[name]; !ok {
			c.addErrorWithCode(node, "TS2343", fmt.Sprintf("This syntax requires an imported helper named '%s' which does not exist in 'tslib'. Consider upgrading your version of 'tslib'.", name))
		}
	}
}

// emitsBeforeSystem reports whether the file is emitted as CommonJS, AMD or
// UMD (tsc's getEmitModuleFormatOfFile < ModuleKind.System).
func (c *Checker) emitsBeforeSystem() bool {
	if c.nodeModuleResolution {
		return !nodemodules.IsESMFile(c.currentFilePath())
	}
	switch c.legacyModuleFormat {
	case "CommonJS", "AMD", "UMD":
		return true
	}
	return false
}

// checkExternalEmitHelpers checks the helpers the file's syntax needs.
func (c *Checker) checkExternalEmitHelpers(program *parser.Program) {
	if !c.importHelpers || c.inDeclarationFile() || !isExternalModuleProgram(program) {
		return
	}
	c.helperState = &emitHelperState{reported: map[string]bool{}}
	cjs := c.emitsBeforeSystem()
	interop := cjs && !c.esModuleInteropOff
	for _, stmt := range program.Statements {
		switch s := stmt.(type) {
		case *parser.ImportDeclaration:
			if s.IsTypeOnly || !interop {
				continue
			}
			for _, spec := range s.Specifiers {
				switch sp := spec.(type) {
				case *parser.ImportNamedSpecifier:
					if !sp.IsTypeOnly && sp.Imported != nil && sp.Imported.Value == "default" {
						c.requireHelpers(sp.Imported, "__importDefault")
					}
				case *parser.ImportNamespaceSpecifier:
					c.requireHelpers(s, "__importStar")
				}
			}
		case *parser.ExportNamedDeclaration:
			if s.Source == nil || s.IsTypeOnly || !interop {
				continue
			}
			for _, spec := range s.Specifiers {
				if es, ok := spec.(*parser.ExportNamedSpecifier); ok && !es.IsTypeOnly {
					if id, ok := es.Local.(*parser.Identifier); ok && id.Value == "default" {
						c.requireHelpers(es.Local, "__importDefault")
					}
				}
			}
		case *parser.ExportAllDeclaration:
			switch {
			case s.IsTypeOnly || !cjs:
			case s.Exported != nil:
				if interop {
					c.requireHelpers(s, "__importStar")
				}
			default:
				c.requireHelpers(s, "__exportStar")
			}
		}
	}
	c.checkClassEmitHelpers(program)
}

// isExternalModuleProgram reports whether the file has a top-level import or
// export.
func isExternalModuleProgram(program *parser.Program) bool {
	for _, stmt := range program.Statements {
		switch s := stmt.(type) {
		case *parser.ImportDeclaration, *parser.ExportDefaultDeclaration, *parser.ExportAllDeclaration,
			*parser.ExportAssignment, *parser.ExportNamedDeclaration:
			return true
		case *parser.ImportEqualsDeclaration:
			if s.Require != nil || s.IsExported {
				return true
			}
		}
	}
	return false
}

// namedEvaluation says a class expression initializes a named evaluation
// source (tsc's isNamedEvaluationSource): it takes the name of what it
// initializes, which ES decorator emit does with __setFunctionName.
type namedEvaluation struct{ computedKey bool }

// checkClassEmitHelpers checks the decorator helpers of every class in the
// file, found by walking the syntax tree. The walk is by reflection because
// it runs only under --importHelpers.
func (c *Checker) checkClassEmitHelpers(program *parser.Program) {
	named := map[*parser.ClassExpression]namedEvaluation{}
	var classes []parser.Node
	seen := map[uintptr]bool{}
	mark := func(e parser.Expression, computed bool) {
		if ce, ok := e.(*parser.ClassExpression); ok && ce.Name == nil {
			named[ce] = namedEvaluation{computedKey: computed}
		}
	}
	isComputed := func(e parser.Expression) bool {
		_, ok := e.(*parser.ComputedPropertyName)
		return ok
	}
	var walk func(v reflect.Value)
	walk = func(v reflect.Value) {
		switch v.Kind() {
		case reflect.Interface:
			if !v.IsNil() {
				walk(v.Elem())
			}
		case reflect.Slice:
			for i := 0; i < v.Len(); i++ {
				walk(v.Index(i))
			}
		case reflect.Ptr:
			if v.IsNil() || !strings.HasSuffix(v.Type().Elem().PkgPath(), "/parser") || seen[v.Pointer()] {
				return
			}
			seen[v.Pointer()] = true
			switch n := v.Interface().(type) {
			case *parser.ClassDeclaration:
				classes = append(classes, n)
			case *parser.ClassExpression:
				classes = append(classes, n)
			case *parser.VarDeclarator:
				if n.Name != nil {
					mark(n.Value, false)
				}
			case *parser.AssignmentExpression:
				if _, ok := n.Left.(*parser.Identifier); ok {
					switch n.Operator {
					case "=", "&&=", "||=", "??=":
						mark(n.Value, false)
					}
				}
			case *parser.Parameter:
				if n.Name != nil {
					mark(n.DefaultValue, false)
				}
			case *parser.DestructuringElement:
				if _, ok := n.Target.(*parser.Identifier); ok && !n.IsRest {
					mark(n.Default, false)
				}
			case *parser.DestructuringProperty:
				if _, ok := n.Target.(*parser.Identifier); ok {
					mark(n.Default, false)
				}
			case *parser.ObjectProperty:
				if _, spread := n.Key.(*parser.SpreadElement); !spread {
					mark(n.Value, isComputed(n.Key))
				}
			case *parser.PropertyDefinition:
				mark(n.Value, isComputed(n.Key))
			case *parser.ExportDefaultDeclaration:
				if !n.IsDeclaration {
					mark(n.Declaration, false)
				}
			}
			walk(v.Elem())
		case reflect.Struct:
			for i := 0; i < v.NumField(); i++ {
				if v.Type().Field(i).IsExported() {
					walk(v.Field(i))
				}
			}
		}
	}
	walk(reflect.ValueOf(program))
	for _, cls := range classes {
		switch n := cls.(type) {
		case *parser.ClassDeclaration:
			c.checkClassHelpers(n, n.Name == nil, n.Decorators, n.Body, nil)
		case *parser.ClassExpression:
			ne, isNamed := named[n]
			if !isNamed {
				c.checkClassHelpers(n, false, n.Decorators, n.Body, nil)
			} else {
				c.checkClassHelpers(n, false, n.Decorators, n.Body, &ne)
			}
		}
	}
}

// isPrivateName reports whether a class member key is a private name.
func isPrivateName(key parser.Expression) bool {
	id, ok := key.(*parser.Identifier)
	return ok && strings.HasPrefix(id.Value, "#")
}

// checkClassHelpers checks the helpers one class's decorators, and the
// elements decorator emit has to transform, need. named is set for an
// anonymous class expression that takes its name from where it is used.
func (c *Checker) checkClassHelpers(cls parser.Node, nameless bool, decorators []*parser.Decorator, body *parser.ClassBody, named *namedEvaluation) {
	if body == nil {
		return
	}
	_, isExpr := cls.(*parser.ClassExpression)
	legacy := c.experimentalDecorators
	// ES decorators are emitted natively only for target ESNext.
	transform := !legacy && c.target() < 9999
	classDecorated := len(decorators) > 0

	// firstTransformableStatic is tsc's getFirstTransformableStaticClassElement.
	firstTransformableStatic := func() parser.Node {
		var loc parser.Node = cls
		if classDecorated {
			loc = decorators[0]
		}
		decoratedMember := func(ds []*parser.Decorator) bool { return transform && classDecorated && len(ds) > 0 }
		transformPrivate := c.target() < 9999
		for _, m := range body.Methods {
			if decoratedMember(m.Decorators) {
				return loc
			}
			if transformPrivate && m.IsStatic && isPrivateName(m.Key) {
				return m
			}
		}
		for _, p := range body.Properties {
			if decoratedMember(p.Decorators) {
				return loc
			}
			if transformPrivate && p.IsStatic && (isPrivateName(p.Key) || c.noStandardClassFields && p.Value != nil) {
				return p
			}
		}
		if transformPrivate && len(body.StaticInitializers) > 0 {
			return body.StaticInitializers[0]
		}
		return nil
	}

	if classDecorated {
		switch {
		case legacy:
			c.requireHelpers(decorators[0], "__decorate")
		case transform:
			c.requireHelpers(decorators[0], "__esDecorate", "__runInitializers")
			if !isExpr {
				if nameless || firstTransformableStatic() != nil {
					c.requireHelpers(decorators[0], "__setFunctionName")
				}
			}
		}
	}
	if isExpr && named != nil && transform {
		var loc parser.Node
		if classDecorated {
			loc = decorators[0]
		} else {
			loc = firstTransformableStatic()
		}
		if loc != nil {
			c.requireHelpers(loc, "__setFunctionName")
			if named.computedKey {
				c.requireHelpers(loc, "__propKey")
			}
		}
	}

	member := func(ds []*parser.Decorator, key parser.Expression, privateNeedsName bool) {
		if len(ds) == 0 {
			return
		}
		switch {
		case legacy:
			c.requireHelpers(ds[0], "__decorate")
		case transform:
			c.requireHelpers(ds[0], "__esDecorate", "__runInitializers")
			if isPrivateName(key) && privateNeedsName {
				c.requireHelpers(ds[0], "__setFunctionName")
			}
			if _, ok := key.(*parser.ComputedPropertyName); ok {
				c.requireHelpers(ds[0], "__propKey")
			}
		}
	}
	for _, m := range body.Methods {
		member(m.Decorators, m.Key, m.Kind != "constructor")
	}
	for _, p := range body.Properties {
		member(p.Decorators, p.Key, p.IsAccessor)
	}
}
