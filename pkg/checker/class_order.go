package checker

import (
	"sort"

	"github.com/nooga/paserati/pkg/parser"
	"github.com/nooga/paserati/pkg/types"
)

// orderInstanceMembersBySource puts the instance type's property order in the
// order the members were written in the class body. The checker adds
// accessors, methods and fields in separate passes, so insertion order alone
// does not match the source.
func (c *Checker) orderInstanceMembersBySource(instanceType *types.ObjectType, body *parser.ClassBody) {
	pos := make(map[string]int)
	note := func(name string, at int) {
		if prev, ok := pos[name]; !ok || at < prev {
			pos[name] = at
		}
	}
	for _, m := range body.Methods {
		if m.Token != nil && !m.IsStatic && m.Kind != "constructor" {
			note(c.extractPropertyName(m.Key), m.Token.StartPos)
		}
	}
	for _, p := range body.Properties {
		if p.Token != nil && !p.IsStatic {
			note(c.extractPropertyName(p.Key), p.Token.StartPos)
		}
	}
	names := instanceType.PropertyNames()
	sort.SliceStable(names, func(i, j int) bool {
		pi, iok := pos[names[i]]
		pj, jok := pos[names[j]]
		if !iok || !jok {
			return false
		}
		return pi < pj
	})
	instanceType.PropertyOrder = names
}

// attachClassMemberDocs records member JSDoc on the instance type and, for
// static members, on the constructor type. An accessor pair is documented by
// whichever of getter and setter carries a comment (getter first).
func (c *Checker) attachClassMemberDocs(body *parser.ClassBody, instanceType, constructorType *types.ObjectType) {
	set := func(isStatic bool, name, doc string) {
		if doc == "" {
			return
		}
		target := instanceType
		if isStatic {
			target = constructorType
		}
		if target == nil {
			return
		}
		if _, exists := target.PropertyDocs[name]; exists && doc != "" {
			// keep the first (getter before setter) documented declaration
			return
		}
		target.SetPropertyDoc(name, doc)
	}
	for _, m := range body.Methods {
		if m.Token != nil && m.Kind != "constructor" {
			set(m.IsStatic, c.extractPropertyName(m.Key), m.Doc)
		}
	}
	for _, p := range body.Properties {
		if p.Token != nil {
			set(p.IsStatic, c.extractPropertyName(p.Key), p.Doc)
		}
	}
}
