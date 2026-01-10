package checker

import (
	"fmt"

	"github.com/nooga/paserati/pkg/parser"
	"github.com/nooga/paserati/pkg/types"
)

// checkEnumDeclaration handles type checking for enum declarations
func (c *Checker) checkEnumDeclaration(node *parser.EnumDeclaration) {
	debugPrintf("// [Checker Enum] Checking enum declaration '%s' (const=%v)\n", node.Name.Value, node.IsConst)

	// 1. Check if enum name is already defined
	if _, _, exists := c.env.Resolve(node.Name.Value); exists {
		c.addError(node.Name, fmt.Sprintf("identifier '%s' already declared", node.Name.Value))
		return
	}

	// 2. Create enum type and member types
	enumType := &types.EnumType{
		Name:      node.Name.Value,
		Members:   make(map[string]*types.EnumMemberType),
		IsConst:   node.IsConst,
		IsNumeric: true, // Assume numeric until we find a string
	}

	// 3. Process enum members
	nextValue := 0                       // For auto-increment
	memberNames := make(map[string]bool) // Check for duplicates
	lastMemberWasNumeric := true         // Track if the previous member was numeric

	for _, member := range node.Members {
		memberName := member.Name.Value

		// Check for duplicate member names
		if memberNames[memberName] {
			c.addError(member.Name, fmt.Sprintf("duplicate identifier '%s'", memberName))
			continue
		}
		memberNames[memberName] = true

		// Determine member value
		var memberValue interface{}
		var isThisMemberNumeric bool

		if member.Value != nil {
			// Member has explicit initializer
			c.visit(member.Value)

			// Handle different initializer types
			switch v := member.Value.(type) {
			case *parser.NumberLiteral:
				memberValue = int(v.Value)
				nextValue = int(v.Value) + 1
				isThisMemberNumeric = true
			case *parser.StringLiteral:
				memberValue = v.Value
				enumType.IsNumeric = false
				isThisMemberNumeric = false
				// String members don't affect auto-increment
			case *parser.PrefixExpression:
				// Handle negative numbers
				if v.Operator == "-" {
					if numLit, ok := v.Right.(*parser.NumberLiteral); ok {
						memberValue = -int(numLit.Value)
						nextValue = -int(numLit.Value) + 1
						isThisMemberNumeric = true
					} else {
						c.addError(member.Value, "enum member must have initializer")
						continue
					}
				} else {
					c.addError(member.Value, "enum member must have initializer")
					continue
				}
			default:
				// For now, only support literal values
				c.addError(member.Value, "enum member initializer must be a constant expression")
				continue
			}
		} else {
			// No initializer - use auto-increment
			if !lastMemberWasNumeric {
				// Can't auto-increment after string member
				c.addError(member.Name, "enum member must have initializer")
				continue
			}
			memberValue = nextValue
			nextValue++
			isThisMemberNumeric = true
		}

		// Create enum member type
		memberType := &types.EnumMemberType{
			EnumName:   node.Name.Value,
			MemberName: memberName,
			Value:      memberValue,
		}

		enumType.Members[memberName] = memberType

		// Note: EnumMember doesn't need computed type as it's not an expression

		debugPrintf("// [Checker Enum] Added member '%s' = %v\n", memberName, memberValue)

		// Update tracking for next iteration
		lastMemberWasNumeric = isThisMemberNumeric
	}

	// 4. Create a union type of all member types for the enum type context
	var memberTypes []types.Type
	for _, member := range enumType.Members {
		memberTypes = append(memberTypes, member)
	}

	// 5. Register enum as both type and value (following class pattern)

	// First, define a forward reference to handle self-references
	forwardRef := &types.ForwardReferenceType{
		ClassName: node.Name.Value, // Reusing ClassName field for enum name
	}
	c.env.Define(node.Name.Value, forwardRef, false)

	// Define the enum name as a type alias to the union of its members
	unionType := &types.UnionType{Types: memberTypes}
	if !c.env.DefineTypeAlias(node.Name.Value, unionType) {
		c.addError(node.Name, fmt.Sprintf("failed to define enum type '%s'", node.Name.Value))
		return
	}

	// Update the value binding to be the enum type itself
	// (This allows both type and value contexts to work)
	if !c.env.Update(node.Name.Value, enumType) {
		c.addError(node.Name, fmt.Sprintf("failed to update enum '%s'", node.Name.Value))
		return
	}

	// 6. For const enums, mark them in a special registry (for compile-time inlining)
	if node.IsConst {
		// TODO: Add const enum registry for compiler optimization
		debugPrintf("// [Checker Enum] Marked '%s' as const enum\n", node.Name.Value)
	}

	// Set the computed type on the enum declaration itself
	node.SetComputedType(enumType)

	debugPrintf("// [Checker Enum] Successfully defined enum '%s' with %d members\n",
		node.Name.Value, len(enumType.Members))
}
