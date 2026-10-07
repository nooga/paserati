package checker

import (
	"fmt"
	"strings"

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
				c.checkPropertyAgainstIndexSignatures(own, prop.ComputedName, name, own.Properties[name], sigs)
			}
			continue
		}
		if prop.Name == nil {
			continue
		}
		c.checkPropertyAgainstIndexSignatures(own, prop.Name, prop.Name.Value, own.Properties[prop.Name.Value], sigs)
	}
}

// checkInterfaceExtends reports TS2430 when an interface is not assignable to
// one of the interfaces it extends (its own members are incompatible with the
// inherited ones).
func (c *Checker) checkInterfaceExtends(node *parser.InterfaceDeclaration, own *types.ObjectType, extended []*types.ObjectType) {
	if own == nil || node.Name == nil {
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
			if strings.Contains(baseName, "<") && strings.Contains(baseName, node.Name.Value) {
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
	props := make(map[string]types.Type, len(t.Properties))
	for name, pt := range t.Properties {
		if name == "new" || name == "__call" {
			continue
		}
		props[name] = pt
	}
	return &types.ObjectType{
		Properties:         props,
		OptionalProperties: t.OptionalProperties,
		ReadOnlyProperties: t.ReadOnlyProperties,
		IndexSignatures:    t.IndexSignatures,
		BaseTypes:          t.BaseTypes,
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
		for _, es := range ext.IndexSignatures {
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
