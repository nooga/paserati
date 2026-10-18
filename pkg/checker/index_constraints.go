package checker

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/nooga/paserati/pkg/errors"
	"github.com/nooga/paserati/pkg/lexer"
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
	c.checkPropertyAgainstIndexSignaturesNamed(owner, nameNode, name, name, propType, sigs)
}

// checkPropertyAgainstIndexSignaturesNamed is checkPropertyAgainstIndexSignatures
// for a property written as name whose property name is key (`2.0` is "2").
func (c *Checker) checkPropertyAgainstIndexSignaturesNamed(owner *types.ObjectType, nameNode parser.Node, name, key string, propType types.Type, sigs []*types.IndexSignature) {
	if propType == nil || nameNode == nil {
		return
	}
	for _, sig := range sigs {
		if sig == nil || sig.ValueType == nil || sig.IsMapped || sig.Synthetic || !indexSignatureApplies(sig.KeyType, key) {
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
		c.checkPropertyAgainstIndexSignaturesNamed(own, prop.Name, prop.Name.Value, numericLiteralKey(prop.Name), own.Props()[prop.Name.Value], sigs)
	}
}

// checkInterfaceExtends reports TS2430 when an interface is not assignable to
// one of the interfaces it extends (its own members are incompatible with the
// inherited ones).
func (c *Checker) checkInterfaceExtends(node *parser.InterfaceDeclaration, own *types.ObjectType, extended []*types.ObjectType) {
	if own == nil || node.Name == nil || c.skipValidation() {
		return
	}
	name := node.Name.Value
	if len(node.TypeParameters) > 0 {
		params := make([]string, 0, len(node.TypeParameters))
		for _, tp := range node.TypeParameters {
			if tp != nil && tp.Name != nil {
				params = append(params, tp.Name.Value)
			}
		}
		name += "<" + strings.Join(params, ", ") + ">"
	}
	for i, ext := range extended {
		if ext == nil || (!redeclaresNonPublicMember(node, ext) && c.isAssignableWithExpansion(membersView(own), membersView(ext))) {
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
			"Interface '%s' incorrectly extends interface '%s'.", name, baseName))
	}
}

// redeclaresNonPublicMember reports whether the interface declares a member
// that is private or protected in the class type ext: the member is then
// not the one the class declares, so the interface is not assignable to it.
func redeclaresNonPublicMember(node *parser.InterfaceDeclaration, ext *types.ObjectType) bool {
	for _, p := range node.Properties {
		if p == nil || p.Name == nil || p.IsComputedProperty || p.IsIndexSignature || p.IsConstructorSignature {
			continue
		}
		if _, ok := types.NonPublicMemberOwner(ext, p.Name.Value); ok {
			return true
		}
	}
	return false
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
	if body == nil {
		return
	}
	instanceSigs, staticSigs := classIndexSignatures(instanceType, 0), classIndexSignatures(ctorType, 0)
	if len(instanceSigs) == 0 && len(staticSigs) == 0 {
		return
	}
	check := func(key parser.Expression, static bool) {
		owner, sigs := instanceType, instanceSigs
		if static {
			owner, sigs = ctorType, staticSigs
		}
		if owner == nil || key == nil || len(sigs) == 0 {
			return
		}
		name := c.extractPropertyName(key)
		known := name != ""
		if cp, ok := key.(*parser.ComputedPropertyName); ok {
			name = c.extractConstantPropertyName(cp.Expr)
			_, isStr := cp.Expr.(*parser.StringLiteral)
			known = name != "" || isStr // [""] names the empty string
		}
		if !known {
			return
		}
		propType := owner.Props()[name]
		if propType == nil {
			return
		}
		keyName := name
		if id, ok := key.(*parser.Identifier); ok {
			keyName = numericLiteralKey(id)
		}
		c.checkPropertyAgainstIndexSignaturesNamed(owner, key, name, keyName, propType, sigs)
	}
	// A computed name that is not a literal binds no property; tsc checks
	// the member against the index signatures its key type selects.
	checkUnbound := func(key parser.Expression, tok *lexer.Token, static bool, memberType types.Type) {
		cp, ok := key.(*parser.ComputedPropertyName)
		if !ok || cp.Expr == nil || memberType == nil || c.extractConstantPropertyName(cp.Expr) != "" {
			return
		}
		if _, isStr := cp.Expr.(*parser.StringLiteral); isStr {
			return
		}
		sigs := instanceSigs
		if static {
			sigs = staticSigs
		}
		keyType := cp.Expr.GetComputedType()
		if keyType == nil || len(sigs) == 0 {
			return
		}
		name := ""
		switch {
		case c.isAssignableWithExpansion(keyType, types.Number):
			name = "0" // any numeric name
		case c.isAssignableWithExpansion(keyType, types.String):
		default:
			return
		}
		applicable := make([]*types.IndexSignature, 0, len(sigs))
		for _, sig := range sigs {
			if sig != nil && indexSignatureApplies(sig.KeyType, name) {
				applicable = append(applicable, sig)
			}
		}
		for _, sig := range applicable {
			if sig.ValueType == nil || sig.IsMapped || sig.Synthetic || c.isAssignableWithExpansion(memberType, sig.ValueType) {
				continue
			}
			var at parser.Node = key
			if tok != nil && tok.Literal == "[" {
				at = &parser.Identifier{Token: tok, Value: "["}
			}
			c.addErrorAtStart(at, errors.TS2411, fmt.Sprintf(
				"Property '[%s]' of type '%s' is not assignable to '%s' index type '%s'.",
				computedNameText(cp.Expr), memberType.String(), sig.KeyType.String(), sig.ValueType.String()))
		}
	}
	declared := map[string]bool{}
	for _, prop := range body.Properties {
		if prop != nil && !isPrivateNameKey(prop.Key) {
			check(prop.Key, prop.IsStatic)
			if !prop.IsStatic {
				declared[c.classMemberKeyName(prop.Key)] = true
			}
			if prop.TypeAnnotation != nil {
				checkUnbound(prop.Key, prop.Token, prop.IsStatic, c.resolveTypeAnnotation(prop.TypeAnnotation))
			}
		}
	}
	for _, m := range body.Methods {
		if m != nil && m.Kind != "constructor" && !isPrivateNameKey(m.Key) {
			check(m.Key, m.IsStatic)
			if !m.IsStatic {
				declared[c.classMemberKeyName(m.Key)] = true
			}
			if m.Value != nil {
				if fn, ok := m.Value.GetComputedType().(*types.ObjectType); ok && len(fn.CallSigs()) > 0 {
					sig := fn.CallSigs()[0]
					switch m.Kind {
					case "getter":
						checkUnbound(m.Key, m.Token, m.IsStatic, sig.ReturnType)
					case "setter":
						if len(sig.ParameterTypes) > 0 {
							checkUnbound(m.Key, m.Token, m.IsStatic, sig.ParameterTypes[0])
						}
					default:
						checkUnbound(m.Key, m.Token, m.IsStatic, fn)
					}
				}
			}
		}
	}
	c.checkInheritedAgainstOwnIndexSignatures(body, instanceType, declared)
}

// numericLiteralKey is the property name a member name binds: a numeric
// literal names its numeric value (`2.0` is "2").
func numericLiteralKey(id *parser.Identifier) string {
	if id.Token != nil && id.Token.Type == lexer.NUMBER {
		if f, err := strconv.ParseFloat(strings.ReplaceAll(id.Value, "_", ""), 64); err == nil {
			return strconv.FormatFloat(f, 'g', -1, 64)
		}
	}
	return id.Value
}

// computedNameText spells a computed name's expression roughly as written.
func computedNameText(e parser.Expression) string {
	switch x := e.(type) {
	case *parser.InfixExpression:
		return computedNameText(x.Left) + " " + x.Operator + " " + computedNameText(x.Right)
	case *parser.Identifier:
		return x.Value
	}
	if text := entityNameText(e); text != "" {
		return text
	}
	if n, ok := e.(*parser.NumberLiteral); ok && n.Token != nil {
		return n.Token.Literal
	}
	return e.String()
}

// classMemberKeyName is the property name a class member key binds, or "".
func (c *Checker) classMemberKeyName(key parser.Expression) string {
	if cp, ok := key.(*parser.ComputedPropertyName); ok {
		return c.extractConstantPropertyName(cp.Expr)
	}
	return c.extractPropertyName(key)
}

// checkInheritedAgainstOwnIndexSignatures checks the instance properties a
// class inherits, and does not redeclare, against the index signatures it
// declares itself, reporting at the index signature (tsc's
// checkIndexConstraintForProperty).
func (c *Checker) checkInheritedAgainstOwnIndexSignatures(body *parser.ClassBody, instanceType *types.ObjectType, declared map[string]bool) {
	if instanceType == nil || len(body.IndexSignatures) == 0 {
		return
	}
	for _, decl := range body.IndexSignatures {
		if decl == nil || decl.IsStatic || decl.Token == nil {
			continue
		}
		keyType := c.resolveTypeAnnotation(decl.KeyType)
		valueType := c.resolveTypeAnnotation(decl.ValueType)
		if keyType == nil || valueType == nil {
			continue
		}
		for _, base := range instanceType.Bases() {
			bo, ok := base.(*types.ObjectType)
			if !ok {
				continue
			}
			props := bo.GetEffectiveProperties()
			names := make([]string, 0, len(props))
			for name := range props {
				names = append(names, name)
			}
			sort.Strings(names)
			for _, name := range names {
				propType := props[name]
				if declared[name] || propType == nil || strings.HasPrefix(name, "#") || name == "__COMPUTED_PROPERTY__" ||
					!indexSignatureApplies(keyType, name) || c.isAssignableWithExpansion(propType, valueType) {
					continue
				}
				if _, nonPublic := types.NonPublicMemberOwner(bo, name); nonPublic {
					continue
				}
				c.addErrorAtStart(&parser.Identifier{Token: decl.Token, Value: "["}, errors.TS2411, fmt.Sprintf(
					"Property '%s' of type '%s' is not assignable to '%s' index type '%s'.",
					name, propType.String(), keyType.String(), valueType.String()))
			}
		}
	}
}

func isPrivateNameKey(key parser.Expression) bool {
	id, ok := key.(*parser.Identifier)
	return ok && len(id.Value) > 0 && id.Value[0] == '#'
}

// classIndexSignatures is t's own index signatures and those of its bases.
func classIndexSignatures(t *types.ObjectType, depth int) []*types.IndexSignature {
	if t == nil || depth > 32 {
		return nil
	}
	sigs := append([]*types.IndexSignature(nil), t.IndexSigs()...)
	for _, b := range t.Bases() {
		if bo, ok := b.(*types.ObjectType); ok {
			sigs = append(sigs, classIndexSignatures(bo, depth+1)...)
		}
	}
	return sigs
}
