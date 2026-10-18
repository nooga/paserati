package checker

import (
	"reflect"

	"github.com/nooga/paserati/pkg/errors"
	"github.com/nooga/paserati/pkg/parser"
)

// typePositionFields are the AST fields that hold types.
var typePositionFields = map[string]bool{
	"TypeAnnotation": true, "ReturnTypeAnnotation": true, "TypeArguments": true,
	"TargetType": true, "Constraint": true, "DefaultType": true, "Type": true,
	"KeyType": true, "ValueType": true, "SuperTypeArgs": true,
}

// checkStaticTypeParameterRefs reports TS2302 for each type reference to a
// class type parameter inside a static member, where no instance supplies
// the type argument.
func (c *Checker) checkStaticTypeParameterRefs(typeParams []*parser.TypeParameter, body *parser.ClassBody) {
	if len(typeParams) == 0 || body == nil {
		return
	}
	names := map[string]bool{}
	for _, tp := range typeParams {
		if tp != nil && tp.Name != nil {
			names[tp.Name.Value] = true
		}
	}
	report := func(id *parser.Identifier) {
		c.addErrorWithCode(id, errors.TS2302, "Static members cannot reference class type parameters.")
	}
	for _, m := range body.Methods {
		if m != nil && m.IsStatic {
			walkTypeParamRefs(reflect.ValueOf(m), names, false, report)
		}
	}
	for _, m := range body.MethodSigs {
		if m != nil && m.IsStatic {
			walkTypeParamRefs(reflect.ValueOf(m), names, false, report)
		}
	}
	for _, p := range body.Properties {
		if p != nil && p.IsStatic {
			walkTypeParamRefs(reflect.ValueOf(p), names, false, report)
		}
	}
	for _, sig := range body.IndexSignatures {
		if sig != nil && sig.IsStatic {
			walkTypeParamRefs(reflect.ValueOf(sig), names, true, report)
		}
	}
}

func walkTypeParamRefs(v reflect.Value, names map[string]bool, inType bool, report func(*parser.Identifier)) {
	for v.Kind() == reflect.Interface {
		if v.IsNil() {
			return
		}
		v = v.Elem()
	}
	if v.Kind() == reflect.Ptr {
		if v.IsNil() {
			return
		}
		switch n := v.Interface().(type) {
		case *parser.Identifier:
			if inType && names[n.Value] {
				report(n)
			}
			return
		case *parser.ClassExpression, *parser.ClassDeclaration:
			return // a nested class is checked on its own
		case *parser.FunctionLiteral:
			names = withoutShadowed(names, n.TypeParameters)
		case *parser.ArrowFunctionLiteral:
			names = withoutShadowed(names, n.TypeParameters)
		}
		v = v.Elem()
	}
	if v.Kind() != reflect.Struct {
		return
	}
	t := v.Type()
	for i := 0; i < v.NumField(); i++ {
		f := v.Field(i)
		if !f.CanInterface() {
			continue
		}
		fieldInType := inType || typePositionFields[t.Field(i).Name]
		switch f.Kind() {
		case reflect.Ptr, reflect.Interface:
			if f.IsNil() {
				continue
			}
			if _, isNode := f.Interface().(parser.Node); isNode || (f.Kind() == reflect.Ptr && f.Type().Elem().PkgPath() == nodeInterface.PkgPath()) {
				walkTypeParamRefs(f, names, fieldInType, report)
			}
		case reflect.Slice:
			for j := 0; j < f.Len(); j++ {
				e := f.Index(j)
				if (e.Kind() == reflect.Ptr || e.Kind() == reflect.Interface) && !e.IsNil() {
					if _, isNode := e.Interface().(parser.Node); isNode || (e.Kind() == reflect.Ptr && e.Type().Elem().PkgPath() == nodeInterface.PkgPath()) {
						walkTypeParamRefs(e, names, fieldInType, report)
					}
				}
			}
		}
	}
}

func withoutShadowed(names map[string]bool, tps []*parser.TypeParameter) map[string]bool {
	if len(tps) == 0 {
		return names
	}
	out := make(map[string]bool, len(names))
	for k := range names {
		out[k] = true
	}
	for _, tp := range tps {
		if tp != nil && tp.Name != nil {
			delete(out, tp.Name.Value)
		}
	}
	return out
}
