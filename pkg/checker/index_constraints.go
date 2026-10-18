package checker

import (
	"fmt"

	"github.com/nooga/paserati/pkg/errors"
	"github.com/nooga/paserati/pkg/parser"
	"github.com/nooga/paserati/pkg/types"
)

// indexSignatureApplies reports whether an index signature with the given key
// type constrains a property with the given (statically known) name
// (getApplicableIndexInfos).
func indexSignatureApplies(keyType types.Type, name string) bool {
	switch keyType {
	case types.String, types.Any:
		return true
	case types.Number:
		return isNumericPropertyName(name)
	}
	return false
}

// checkPropertyAgainstIndexSignatures reports TS2411 for a property whose type
// is not assignable to an applicable index signature
// (checkIndexConstraintForProperty).
func (c *Checker) checkPropertyAgainstIndexSignatures(owner *types.ObjectType, nameNode parser.Node, name string, propType types.Type, sigs []*types.IndexSignature) {
	if propType == nil || nameNode == nil {
		return
	}
	for _, sig := range sigs {
		if sig == nil || sig.ValueType == nil || sig.IsMapped || sig.Synthetic || !indexSignatureApplies(sig.KeyType, name) {
			continue
		}
		if c.isAssignableWithExpansion(propType, sig.ValueType) {
			continue
		}
		// A property merged from several interface declarations is a single
		// symbol, reported once at its first declaration.
		if owner != nil {
			seenKey := fmt.Sprintf("%p:%s:%s", owner, name, sig.KeyType.String())
			if c.indexConstraintsReported == nil {
				c.indexConstraintsReported = make(map[string]bool)
			}
			if c.indexConstraintsReported[seenKey] {
				continue
			}
			c.indexConstraintsReported[seenKey] = true
		}
		c.addErrorAtStart(nameNode, errors.TS2411, fmt.Sprintf(
			"Property '%s' of type '%s' is not assignable to '%s' index type '%s'.",
			name, propType.String(), sig.KeyType.String(), sig.ValueType.String()))
	}
}

// checkInterfaceIndexConstraints checks the properties an interface
// declaration declares itself against the index signatures of the interface
// (its own and those inherited from extended interfaces).
func (c *Checker) checkInterfaceIndexConstraints(node *parser.InterfaceDeclaration, own *types.ObjectType, sigs []*types.IndexSignature) {
	if len(sigs) == 0 || own == nil {
		return
	}
	for _, prop := range node.Properties {
		if prop.IsIndexSignature || prop.IsConstructorSignature {
			continue
		}
		if prop.IsComputedProperty && prop.ComputedName != nil {
			// String/number literal keys and other constant names.
			if name := c.extractConstantPropertyName(prop.ComputedName); name != "" {
				c.checkPropertyAgainstIndexSignatures(own, prop.ComputedName, name, own.Props()[name], sigs)
			}
			continue
		}
		if prop.Name == nil {
			continue
		}
		c.checkPropertyAgainstIndexSignatures(own, prop.Name, prop.Name.Value, own.Props()[prop.Name.Value], sigs)
	}
}

// checkInterfaceExtends reports TS2430 when an interface is not assignable to
// one of the interfaces it extends (its own members are incompatible with the
// inherited ones).
func (c *Checker) checkInterfaceExtends(node *parser.InterfaceDeclaration, own *types.ObjectType, extended []*types.ObjectType) {
	if own == nil || node.Name == nil || c.skipValidation() {
		return
	}
	for i, ext := range extended {
		if ext == nil || c.isAssignableWithExpansion(membersView(own), membersView(ext)) {
			continue
		}
		baseName := ext.String()
		if i < len(node.Extends) && node.Extends[i] != nil {
			baseName = node.Extends[i].String()
			// `interface B extends A<A<B>>`: the instantiation refers back to
			// the interface being declared, so its expansion is not modelled
			// (tsc reports nothing here).
			if ref, ok := node.Extends[i].(*parser.GenericTypeRef); ok && typeArgsReference(ref.TypeArguments, node.Name.Value) {
				continue
			}
		}
		c.addErrorAtStart(node.Name, errors.TS2430, fmt.Sprintf(
			"Interface '%s' incorrectly extends interface '%s'.", node.Name.Value, baseName))
	}
}

// membersView is an object type holding only the property and index members
// of t. Call and construct signatures are left out because an interface
// inherits its bases' signatures (its own are added to them, never required
// to subsume them), so they cannot make it incompatible with a base.
func membersView(t *types.ObjectType) *types.ObjectType {
	props := make(map[string]types.Type, len(t.Props()))
	for name, pt := range t.Props() {
		if name == "new" || name == "__call" {
			continue
		}
		props[name] = pt
	}
	return &types.ObjectType{
		Properties:         props,
		OptionalProperties: t.OptionalProps(),
		ReadOnlyProperties: t.ReadOnlyProps(),
		IndexSignatures:    t.IndexSigs(),
		BaseTypes:          t.Bases(),
	}
}

// appendNewSignatures appends the signatures of extra that dst does not
// already contain.
func appendNewSignatures(dst, extra []*types.Signature) []*types.Signature {
	for _, sig := range extra {
		present := false
		for _, d := range dst {
			if d == sig {
				present = true
				break
			}
		}
		if !present {
			dst = append(dst, sig)
		}
	}
	return dst
}

// inheritIndexSignatures appends the index signatures of extended types that
// are not shadowed by one with the same key type already in sigs.
func inheritIndexSignatures(sigs []*types.IndexSignature, extended []*types.ObjectType) []*types.IndexSignature {
	for _, ext := range extended {
		for _, es := range ext.IndexSigs() {
			if es == nil {
				continue
			}
			shadowed := false
			for _, s := range sigs {
				if s != nil && s.KeyType == es.KeyType {
					shadowed = true
					break
				}
			}
			if !shadowed {
				sigs = append(sigs, es)
			}
		}
	}
	return sigs
}

// typeArgsReference reports whether any of the type arguments (at any depth of
// generic nesting) names the given type.
func typeArgsReference(args []parser.Expression, name string) bool {
	for _, a := range args {
		switch t := a.(type) {
		case *parser.Identifier:
			if t.Value == name {
				return true
			}
		case *parser.GenericTypeRef:
			if (t.Name != nil && t.Name.Value == name) || typeArgsReference(t.TypeArguments, name) {
				return true
			}
		}
	}
	return false
}

// addClassIndexSignatures adds a class body's declared index signatures, the
// static ones to the constructor type and the others to the instance type.
func (c *Checker) addClassIndexSignatures(body *parser.ClassBody, target *types.ObjectType, static bool) {
	if body == nil || target == nil {
		return
	}
	var declared []*types.IndexSignature
	for _, sig := range body.IndexSignatures {
		if sig == nil || sig.IsStatic != static {
			continue
		}
		keyType := c.resolveTypeAnnotation(sig.KeyType)
		if keyType == nil {
			keyType = types.Any
		}
		valueType := c.resolveTypeAnnotation(sig.ValueType)
		if valueType == nil {
			valueType = types.Any
		}
		c.reportDuplicateIndexSignature(sig.KeyName, declared, keyType)
		declared = append(declared, &types.IndexSignature{KeyType: keyType, ValueType: valueType, Readonly: sig.Readonly})
	}
	if len(declared) == 0 {
		return
	}
	target.Materialize()
	target.IndexSignatures = append(target.IndexSigs(), declared...)
}

func hasDeclaredStringIndex(t *types.ObjectType) bool {
	for _, sig := range t.IndexSigs() {
		if !sig.Synthetic && sig.KeyType == types.String {
			return true
		}
	}
	return false
}

// checkClassIndexConstraints checks a class's own members against its index
// signatures (checkIndexConstraints): instance members against the instance
// type's, static members against the constructor's.
func (c *Checker) checkClassIndexConstraints(body *parser.ClassBody, instanceType, ctorType *types.ObjectType) {
	if body == nil || len(body.IndexSignatures) == 0 {
		return
	}
	check := func(key parser.Expression, static bool) {
		owner := instanceType
		if static {
			owner = ctorType
		}
		if owner == nil || key == nil {
			return
		}
		name := c.extractPropertyName(key)
		if cp, ok := key.(*parser.ComputedPropertyName); ok {
			name = c.extractConstantPropertyName(cp.Expr)
		}
		if name == "" {
			return
		}
		propType := owner.Props()[name]
		if propType == nil {
			return
		}
		c.checkPropertyAgainstIndexSignatures(owner, key, name, propType, owner.IndexSigs())
	}
	for _, prop := range body.Properties {
		if prop != nil && !isPrivateNameKey(prop.Key) {
			check(prop.Key, prop.IsStatic)
		}
	}
	for _, m := range body.Methods {
		if m != nil && m.Kind != "constructor" && !isPrivateNameKey(m.Key) {
			check(m.Key, m.IsStatic)
		}
	}
}

func isPrivateNameKey(key parser.Expression) bool {
	id, ok := key.(*parser.Identifier)
	return ok && len(id.Value) > 0 && id.Value[0] == '#'
}
