package parser

import "strings"

// declareThisAssignments gives a JavaScript class the instance properties its
// methods create with `this.name = value` statements (tsc's salsa class member
// inference). Their type is any here: the union of the assigned values is not
// inferred yet. They are `declare` fields, so they emit nothing.
func declareThisAssignments(body *ClassBody) {
	declared := map[string]bool{}
	for _, m := range body.Methods {
		if id, ok := m.Key.(*Identifier); ok && !m.IsStatic {
			declared[id.Value] = true
		}
	}
	for _, pd := range body.Properties {
		if id, ok := pd.Key.(*Identifier); ok && !pd.IsStatic {
			declared[id.Value] = true
		}
	}
	var add []*PropertyDefinition
	for _, m := range body.Methods {
		if m.IsStatic || m.Value == nil || m.Value.Body == nil {
			continue
		}
		for _, st := range m.Value.Body.Statements {
			es, ok := st.(*ExpressionStatement)
			if !ok || es == nil {
				continue
			}
			as, ok := es.Expression.(*AssignmentExpression)
			if !ok || as.Operator != "=" {
				continue
			}
			me, ok := as.Left.(*MemberExpression)
			if !ok {
				continue
			}
			if _, ok := me.Object.(*ThisExpression); !ok {
				continue
			}
			id, ok := me.Property.(*Identifier)
			if !ok || declared[id.Value] || strings.HasPrefix(id.Value, "#") {
				continue
			}
			declared[id.Value] = true
			anyTok := *id.Token
			anyTok.Literal = "any"
			add = append(add, &PropertyDefinition{
				Token:          id.Token,
				Key:            id,
				TypeAnnotation: &Identifier{Token: &anyTok, Value: "any"},
				IsDeclare:      true,
				IsPublic:       true,

				FromThisAssignment: true,
			})
		}
	}
	body.Properties = append(body.Properties, add...)
}
