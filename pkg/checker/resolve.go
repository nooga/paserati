package checker

import (
	"fmt"
	"maps"
	"sort"
	"strings"

	"github.com/nooga/paserati/pkg/errors"
	"github.com/nooga/paserati/pkg/lexer"
	"github.com/nooga/paserati/pkg/parser"
	"github.com/nooga/paserati/pkg/types"
	"github.com/nooga/paserati/pkg/vm"
)

// --- Helper Functions ---

// checkTypeRefDefined walks a type expression and emits TS2304 for any type name
// that is not defined in the current environment. Used for strict contexts like
// generic class/interface constraints where forward references are not allowed.
func (c *Checker) checkTypeRefDefined(node parser.Expression) {
	if node == nil {
		return
	}
	switch n := node.(type) {
	case *parser.Identifier:
		// Skip built-in primitive names and type parameters
		if _, found := c.env.ResolveTypeParameter(n.Value); found {
			return
		}
		if _, found := c.env.ResolveType(n.Value); found {
			return
		}
		if _, _, found := c.env.Resolve(n.Value); found {
			return
		}
		// Check primitives
		switch n.Value {
		case "string", "number", "boolean", "any", "void", "never", "unknown",
			"null", "undefined", "object", "symbol", "bigint":
			return
		}
		c.addCannotFindNameError(n, c.env, n.Value)
	case *parser.GenericTypeRef:
		// Check the base name
		c.checkTypeRefDefined(n.Name)
		// Check type arguments recursively
		for _, arg := range n.TypeArguments {
			c.checkTypeRefDefined(arg)
		}
	}
}

// resolveTypeAnnotation converts a parser node representing a type annotation
// into a types.Type representation.
func (c *Checker) resolveTypeAnnotation(node parser.Expression) types.Type {
	t := c.resolveTypeAnnotationNode(node)
	if t != nil {
		t = c.resolveDeferredTypeofIndex(t, false)
	}
	// Parts left deferred by an alias resolved before the classes it mentions
	// are evaluated where the alias is used.
	return c.settleType(t)
}

// settleType re-evaluates the parts of t that were deferred because their
// inputs did not exist yet: aliases resolve in Pass 1, before classes are
// filled in, so `ZodNumber["_output"]` met the empty class placeholder and
// `ZodType extends ZodTypeAny ? ...` met no ZodType at all. The placeholders
// are filled in place, so by the time a type is used these compute. It walks
// unions, intersections, indexed accesses, conditionals and forward
// references; object members are settled where they are read.
func (c *Checker) settleType(t types.Type) types.Type {
	return c.settleTypeDepth(t, 0)
}

func (c *Checker) settleTypeDepth(t types.Type, depth int) types.Type {
	if t == nil || depth > 16 {
		return t
	}
	switch tt := t.(type) {
	case *types.IndexedAccessType:
		obj := c.settleTypeDepth(tt.ObjectType, depth+1)
		idx := c.settleTypeDepth(tt.IndexType, depth+1)
		_, objIsParam := obj.(*types.TypeParameterType)
		_, idxIsParam := idx.(*types.TypeParameterType)
		if !objIsParam && !idxIsParam {
			if resolved := c.computeIndexedAccessType(obj, idx); resolved != nil {
				return c.settleTypeDepth(resolved, depth+1)
			}
		}
		if obj != tt.ObjectType || idx != tt.IndexType {
			return &types.IndexedAccessType{ObjectType: obj, IndexType: idx}
		}
	case *types.ConditionalType:
		check := c.settleTypeDepth(tt.CheckType, depth+1)
		ext := c.settleTypeDepth(tt.ExtendsType, depth+1)
		if resolved := c.computeConditionalType(check, ext, tt.TrueType, tt.FalseType); resolved != nil {
			if _, still := resolved.(*types.ConditionalType); !still {
				return c.settleTypeDepth(resolved, depth+1)
			}
		}
	case *types.GenericTypeAliasForwardReference:
		if inst := c.instantiateAliasReference(tt); inst != nil {
			return c.settleTypeDepth(inst, depth+1)
		}
	case *types.UnionType:
		changed := false
		members := make([]types.Type, len(tt.Types))
		for i, m := range tt.Types {
			members[i] = c.settleTypeDepth(m, depth+1)
			changed = changed || members[i] != m
		}
		if changed {
			return types.NewUnionType(members...)
		}
	case *types.IntersectionType:
		changed := false
		members := make([]types.Type, len(tt.Types))
		for i, m := range tt.Types {
			members[i] = c.settleTypeDepth(m, depth+1)
			changed = changed || members[i] != m
		}
		if changed {
			return types.NewIntersectionType(members...)
		}
	case *types.KeyofType:
		if tof, ok := tt.OperandType.(*types.TypeofType); ok {
			// `keyof typeof C` written before C was declared.
			if r := c.resolveTypeofTypeIfNeeded(tof); r != types.Any {
				if _, still := r.(*types.TypeofType); !still {
					tt.OperandType = r // in place: signatures captured it
					return c.computeKeyofType(r)
				}
			}
			return t
		}
		// Stays a keyof, so a mapped type over it remains homomorphic.
		if operand := c.settleTypeDepth(tt.OperandType, depth+1); operand != tt.OperandType {
			return &types.KeyofType{OperandType: operand}
		}
	case *types.MappedType:
		// The key set may be a deferred `{ ... }[keyof T]` (zod's requiredKeys).
		if constraint := c.settleTypeDepth(tt.ConstraintType, depth+1); constraint != tt.ConstraintType {
			cp := *tt
			cp.ConstraintType = constraint
			return &cp
		}
	case *types.ObjectType:
		// Only properties that are themselves deferred are settled, so an
		// ordinary (possibly recursive) class type is returned as is.
		var settled map[string]types.Type
		for _, name := range tt.PropertyNames() {
			prop := tt.Props()[name]
			// Forward references in members (`owner: OwnerList<T>`) resolve
			// where they are read; copying a recursive interface to settle
			// them would lose its identity.
			if !isDeferredComputation(prop) {
				continue
			}
			if np := c.settleTypeDepth(prop, depth+1); np != prop {
				if settled == nil {
					settled = map[string]types.Type{}
				}
				settled[name] = np
			}
		}
		if settled != nil {
			return cloneObjectTypeWithTypes(tt, func(p types.Type) types.Type {
				for name, np := range settled {
					if tt.Props()[name] == p {
						return np
					}
				}
				return p
			})
		}
	}
	return t
}

// reachesPending reports whether a class or interface placeholder not filled
// in yet is reachable from t through members, base types, type arguments or
// union and intersection members (a few levels deep).
func reachesPending(t types.Type, depth int, seen map[types.Type]bool) bool {
	if t == nil || depth > 4 {
		return false
	}
	if seen == nil {
		seen = map[types.Type]bool{}
	}
	if seen[t] {
		return false
	}
	seen[t] = true
	switch tt := t.(type) {
	case *types.ObjectType:
		if tt.Pending {
			return true
		}
		for _, p := range tt.Props() {
			if reachesPending(p, depth+1, seen) {
				return true
			}
		}
		for _, b := range tt.Bases() {
			if reachesPending(b, depth+1, seen) {
				return true
			}
		}
	case *types.UnionType:
		for _, m := range tt.Types {
			if reachesPending(m, depth+1, seen) {
				return true
			}
		}
	case *types.IntersectionType:
		for _, m := range tt.Types {
			if reachesPending(m, depth+1, seen) {
				return true
			}
		}
	case *types.ArrayType:
		return reachesPending(tt.ElementType, depth+1, seen)
	}
	return false
}

// mentionsUndeclared reports whether t refers to a class not filled in yet or
// a generic the program declares later and that cannot be instantiated yet.
func (c *Checker) mentionsUndeclared(t types.Type, depth int) bool {
	if t == nil || depth > 8 {
		return false
	}
	switch tt := t.(type) {
	case *types.ObjectType:
		return tt.Pending
	case *types.GenericTypeAliasForwardReference:
		if !c.declaredTypeNames[tt.AliasName] {
			return false
		}
		_, isGeneric := c.lookupTypeNoPending(tt.AliasName).(*types.GenericType)
		return !isGeneric
	case *types.ParameterizedForwardReferenceType:
		return true
	case *types.IndexedAccessType:
		return c.mentionsUndeclared(tt.ObjectType, depth+1) || c.mentionsUndeclared(tt.IndexType, depth+1)
	case *types.UnionType:
		for _, m := range tt.Types {
			if c.mentionsUndeclared(m, depth+1) {
				return true
			}
		}
	case *types.IntersectionType:
		for _, m := range tt.Types {
			if c.mentionsUndeclared(m, depth+1) {
				return true
			}
		}
	}
	return false
}

// isDeferredComputation reports whether t holds an indexed access or a
// conditional that settleType can evaluate later.
func isDeferredComputation(t types.Type) bool {
	switch tt := t.(type) {
	case *types.IndexedAccessType, *types.ConditionalType:
		return true
	case *types.UnionType:
		for _, m := range tt.Types {
			if isDeferredComputation(m) {
				return true
			}
		}
	case *types.IntersectionType:
		for _, m := range tt.Types {
			if isDeferredComputation(m) {
				return true
			}
		}
	}
	return false
}

// isDeferredType reports whether t is a type settleType can evaluate later.
func isDeferredType(t types.Type) bool {
	switch tt := t.(type) {
	case *types.IndexedAccessType, *types.ConditionalType, *types.GenericTypeAliasForwardReference:
		return true
	case *types.UnionType:
		for _, m := range tt.Types {
			if isDeferredType(m) {
				return true
			}
		}
	case *types.IntersectionType:
		for _, m := range tt.Types {
			if isDeferredType(m) {
				return true
			}
		}
	}
	return false
}

func (c *Checker) resolveTypeAnnotationNode(node parser.Expression) types.Type {
	if node == nil {
		// No annotation provided, perhaps default to any or handle elsewhere?
		// For now, returning nil might be okay, caller decides default.
		return nil
	}

	// Dispatch based on the structure of the type expression node
	switch node := node.(type) {
	case *parser.ThisExpression:
		if c.currentThisType != nil {
			return c.currentThisType
		}
		if c.currentClassInstanceType != nil {
			// A member's declared type: `this` is the receiver's type,
			// substituted where the member is read.
			return &types.TypeParameterType{Parameter: types.PolymorphicThis}
		}
		return types.Any

	case *parser.Identifier:
		// --- ADDED: Check if node or node.Value is nil ---
		if node == nil || node.Value == "" {
			debugPrintf("// [Checker resolveTypeAnno Ident] ERROR: Node (%p) or Node.Value is nil/empty!\n", node)
			return nil // Return nil early if node is bad
		}
		debugPrintf("// [Checker resolveTypeAnno Ident] Processing identifier: '%s'\n", node.Value)
		// --- END ADDED ---

		// --- NEW: Check if this is a type parameter first ---
		if typeParam, found := c.env.ResolveTypeParameter(node.Value); found {
			debugPrintf("// [Checker resolveTypeAnno Ident] Resolved '%s' as type parameter\n", node.Value)
			return &types.TypeParameterType{Parameter: typeParam}
		}

		// --- NEW: Check if this is a forward reference to the current generic class ---
		if c.currentForwardRef != nil && node.Value == c.currentForwardRef.ClassName {
			debugPrintf("// [Checker resolveTypeAnno Ident] Resolved '%s' as forward reference to current generic class\n", node.Value)
			return c.currentForwardRef
		}

		// --- NEW: Check for recursive type alias resolution ---
		if c.resolvingTypeAliases[node.Value] {
			debugPrintf("// [Checker resolveTypeAnno Ident] Detected recursive reference to '%s', creating placeholder\n", node.Value)
			// Create a placeholder type for forward reference
			// This will be resolved later when the full type is available
			return &types.TypeAliasForwardReference{
				AliasName: node.Value,
			}
		}

		// --- PRIORITIZE TYPE ALIASES: Check type alias first before checking value bindings ---
		// This ensures that when a class has both a value binding (ForwardReferenceType for constructor)
		// and a type alias (ObjectType for instance type), we use the type alias for type annotations.
		if resolvedAlias, found := c.env.ResolveType(node.Value); found {
			debugPrintf("// [Checker resolveTypeAnno Ident] Early resolved '%s' as type alias: %T\n", node.Value, resolvedAlias)
			// Check if this is a GenericType that needs instantiation with defaults
			if genericType, ok := resolvedAlias.(*types.GenericType); ok {
				debugPrintf("// [Checker resolveTypeAnno Ident] Found generic type '%s', instantiating with defaults\n", node.Value)
				// Instantiate with no explicit type arguments - will use defaults
				c.typeRefNode = node
				return c.instantiateGenericType(genericType, []types.Type{}, []parser.Expression{})
			}
			return resolvedAlias
		}

		// Special case: 'Object' (capital O) as a type annotation means the Object interface,
		// not the Object constructor. The constructor is callable returning 'any', but in type
		// position we want an empty object type so assignment checking works correctly.
		if node.Value == "Object" {
			return types.NewObjectType()
		}

		// --- Check if this is a forward reference or constructor defined as a regular value ---
		if value, _, found := c.env.Resolve(node.Value); found {
			debugPrintf("// [Checker resolveTypeAnno Ident] Found value for '%s': %T\n", node.Value, value)

			if forwardRef, ok := value.(*types.ForwardReferenceType); ok {
				debugPrintf("// [Checker resolveTypeAnno Ident] Found forward reference for '%s'\n", node.Value)
				return forwardRef
			}

			// Check if this is a constructor type (like Date, Array, String builtins)
			if objType, ok := value.(*types.ObjectType); ok {
				if checkerDebug {
					debugPrintf("// [Checker resolveTypeAnno Ident] '%s' is ObjectType, IsCallable: %v, IsConstructable: %v\n", node.Value, objType.IsCallable(), objType.IsConstructable())
				}

				// Check for construct signatures first (preferred for type annotations)
				if objType.IsConstructable() {
					constructSigs := objType.GetConstructSignatures()
					if checkerDebug {
						debugPrintf("// [Checker resolveTypeAnno Ident] '%s' has %d construct signatures\n", node.Value, len(constructSigs))
					}
					if len(constructSigs) > 0 {
						returnType := constructSigs[0].ReturnType
						debugPrintf("// [Checker resolveTypeAnno Ident] Found constructor type '%s', using return type: %T\n", node.Value, returnType)
						return returnType
					}
				}

				// Check for call signatures (for builtins that are callable as constructors)
				if objType.IsCallable() {
					callSigs := objType.GetCallSignatures()
					if checkerDebug {
						debugPrintf("// [Checker resolveTypeAnno Ident] '%s' has %d call signatures\n", node.Value, len(callSigs))
					}
					if len(callSigs) > 0 {
						returnType := callSigs[0].ReturnType
						debugPrintf("// [Checker resolveTypeAnno Ident] Found callable constructor type '%s', using return type: %T\n", node.Value, returnType)
						return returnType
					}
				}
			} else {
				debugPrintf("// [Checker resolveTypeAnno Ident] '%s' is not ObjectType\n", node.Value)
			}
		}

		// --- UPDATED: Prioritize alias resolution ---
		// 1. Attempt to resolve as a type alias in the environment
		resolvedAlias, found := c.env.ResolveType(node.Value)
		if found {
			debugPrintf("// [Checker resolveTypeAnno Ident] Resolved '%s' as alias: %T\n", node.Value, resolvedAlias) // ADDED DEBUG

			// Check if this is a GenericType that needs instantiation with defaults
			if genericType, ok := resolvedAlias.(*types.GenericType); ok {
				debugPrintf("// [Checker resolveTypeAnno Ident] Found generic type '%s', instantiating with defaults\n", node.Value)
				// Instantiate with no explicit type arguments - will use defaults
				c.typeRefNode = node
				return c.instantiateGenericType(genericType, []types.Type{}, []parser.Expression{})
			}

			return resolvedAlias // Successfully resolved as a non-generic alias
		}
		debugPrintf("// [Checker resolveTypeAnno Ident] '%s' not found as alias, checking primitives...\n", node.Value) // ADDED DEBUG

		// 2. If not found as an alias, check against known primitive type names
		switch node.Value {
		case "number":
			debugPrintf("// [Checker resolveTypeAnno Ident] Matched primitive: number\n") // ADDED DEBUG
			return types.Number                                                           // Returns the package-level variable 'types.Number'
		case "string":
			debugPrintf("// [Checker resolveTypeAnno Ident] Matched primitive: string\n") // ADDED DEBUG
			return types.String
		case "boolean":
			return types.Boolean
		case "null":
			return types.Null
		case "undefined":
			return types.Undefined
		case "any":
			return types.Any
		case "unknown":
			return types.Unknown
		case "never":
			return types.Never
		case "void":
			return types.Void
		case "object":
			return types.NonPrimitive
		case "const":
			// 'as const' const assertion — treat as 'any' since we don't narrow
			// to readonly literal types yet.
			return types.Any
		case "symbol":
			return types.Symbol
		case "bigint":
			return types.BigInt
		// Common DOM / browser types — not in our stdlib but frequently appear in TypeScript
		// parser conformance tests. Treat as 'any' to avoid false TS2304/TS-unknown errors.
		case "HTMLElement", "HTMLCanvasElement", "HTMLDivElement", "HTMLSpanElement",
			"HTMLAnchorElement", "HTMLImageElement", "HTMLInputElement", "HTMLButtonElement",
			"HTMLFormElement", "HTMLTableElement", "HTMLSelectElement", "HTMLTextAreaElement",
			"SVGElement", "SVGSVGElement", "SVGPathElement",
			"Event", "MouseEvent", "KeyboardEvent", "CustomEvent",
			"Node", "Element", "Document", "Window",
			"CSSStyleDeclaration", "DOMTokenList", "NodeList", "HTMLCollection",
			"XMLHttpRequest", "WebSocket", "Worker", "File", "Blob", "URL":
			return types.Any

		default:
			// 3. Check if this is an imported type (interface, type alias, etc.)
			if c.IsModuleMode() && c.moduleEnv != nil {
				importedType := c.moduleEnv.ResolveImportedType(node.Value)
				if importedType != nil && importedType != types.Any {
					debugPrintf("// [Checker resolveTypeAnno Ident] Resolved '%s' as imported type: %T\n", node.Value, importedType)
					return importedType
				}
				debugPrintf("// [Checker resolveTypeAnno Ident] '%s' not found as imported type\n", node.Value)
			}

			// 4. If neither alias, primitive, nor imported type, it's an unknown type name
			debugPrintf("// [Checker resolveTypeAnno Ident] Primitive check failed for '%s', reporting error.\n", node.Value) // ADDED DEBUG
			// Use the Identifier node itself for error reporting
			if c.isLibGlobalValue(node.Value) {
				// lib.d.ts declares an interface of this name (JSON, Atomics,
				// IArguments, ...) that Paserati models only as a value.
				return types.Any
			}
			if c.resolvePendingAlias(node.Value) {
				if t, found := c.env.ResolveType(node.Value); found {
					return t
				}
			}
			if c.declaredTypeNames[node.Value] {
				// A generic declared later whose type parameters all have
				// defaults (`e: ZodError[]` before `class ZodError<T = any>`):
				// a forward reference, instantiated with the defaults on use.
				return &types.GenericTypeAliasForwardReference{AliasName: node.Value, Scope: c.env.ResolveType}
			}
			c.addCannotFindTypeNameError(node, c.env, node.Value)
			return nil // Indicate error
		}

	case *parser.UnionTypeExpression: // Added
		leftType := c.resolveTypeAnnotation(node.Left)
		rightType := c.resolveTypeAnnotation(node.Right)

		if leftType == nil || rightType == nil {
			// Error occurred resolving one of the sides
			return nil
		}

		// --- UPDATED: Use NewUnionType constructor ---
		// This handles flattening and duplicate removal automatically.
		return types.NewUnionType(leftType, rightType)

	case *parser.IntersectionTypeExpression: // NEW: Handle intersection types
		leftType := c.resolveTypeAnnotation(node.Left)
		rightType := c.resolveTypeAnnotation(node.Right)

		if leftType == nil || rightType == nil {
			// Error occurred resolving one of the sides
			return nil
		}

		// Use NewIntersectionType constructor
		// This handles flattening and simplification automatically.
		return types.NewIntersectionType(leftType, rightType)

	// --- NEW: Handle ArrayTypeExpression ---
	case *parser.ArrayTypeExpression:
		elemType := c.resolveTypeAnnotation(node.ElementType)
		if elemType == nil {
			return nil // Error resolving element type
		}
		arrayType := &types.ArrayType{ElementType: elemType}
		// No need to set computed type on the type annotation node itself
		return arrayType

	// --- NEW: Handle TupleTypeExpression ---
	case *parser.TupleTypeExpression:
		elementTypes := []types.Type{}
		seenOptionalElement := false
		for i, elemNode := range node.ElementTypes {
			elemType := c.resolveTypeAnnotation(elemNode)
			if elemType == nil {
				return nil // Error resolving element type
			}
			isOptional := i < len(node.OptionalFlags) && node.OptionalFlags[i]
			if !isOptional && seenOptionalElement {
				c.addError(elemNode, "A required element cannot follow an optional element.")
			}
			if isOptional {
				seenOptionalElement = true
			}
			elementTypes = append(elementTypes, elemType)
		}

		// Handle rest element if present
		var restElementType types.Type
		if node.RestElement != nil {
			restType := c.resolveTypeAnnotation(node.RestElement)
			if restType == nil {
				return nil // Error resolving rest element type
			}

			// Validate that rest element type is an array type
			if _, isArrayType := restType.(*types.ArrayType); !isArrayType {
				c.addError(node.RestElement, fmt.Sprintf("rest element in tuple type must be an array type, got '%s'", restType.String()))
				// Use any[] as fallback
				restElementType = &types.ArrayType{ElementType: types.Any}
			} else {
				restElementType = restType
			}
		}

		tupleType := &types.TupleType{
			ElementTypes:     elementTypes,
			OptionalElements: node.OptionalFlags, // Copy the optional flags directly
			RestElementType:  restElementType,
		}
		return tupleType

	// --- NEW: Handle Literal Type Nodes ---
	case *parser.StringLiteral:
		return &types.LiteralType{Value: vm.String(node.Value)}
	case *parser.NumberLiteral:
		return &types.LiteralType{Value: vm.Number(node.Value)}
	case *parser.BooleanLiteral:
		return &types.LiteralType{Value: vm.BooleanValue(node.Value)}
	case *parser.NullLiteral:
		return types.Null
	case *parser.UndefinedLiteral:
		return types.Undefined
	// Negative numeric literal types: -1, -2.5, etc.
	case *parser.PrefixExpression:
		if node.Operator == "-" {
			if numLit, ok := node.Right.(*parser.NumberLiteral); ok {
				return &types.LiteralType{Value: vm.Number(-numLit.Value)}
			}
		}
		c.addError(node, fmt.Sprintf("unsupported type annotation node: %T", node))
		return nil
	// --- End Literal Type Nodes ---

	// --- NEW: Handle FunctionTypeExpression --- <<<
	case *parser.FunctionTypeExpression:
		return c.resolveFunctionTypeSignature(node)

	// --- NEW: Handle ObjectTypeExpression ---
	case *parser.ObjectTypeExpression:
		return c.resolveObjectTypeSignature(node)

	// --- NEW: Handle GenericTypeRef ---
	case *parser.GenericTypeRef:
		if checkerDebug {
			debugPrintf("// [Checker resolveTypeAnno GenericTypeRef] Processing GenericTypeRef: %s with %d type args\n", node.Name.Value, len(node.TypeArguments))
		}
		// A qualified reference (N.Array<T>) never names a builtin.
		builtinName := node.Name.Value
		if node.Qualifier != nil {
			builtinName = ""
		}
		switch builtinName {
		case "Array":
			if len(node.TypeArguments) != 1 {
				c.addError(node, "Array requires exactly one type argument")
				return nil
			}
			elemType := c.resolveTypeAnnotation(node.TypeArguments[0])
			if elemType == nil {
				return nil // Error already reported
			}
			return &types.ArrayType{ElementType: elemType}

		case "ReadonlyArray":
			// ReadonlyArray<T> is `readonly T[]` (#637).
			if len(node.TypeArguments) != 1 {
				c.addError(node, "ReadonlyArray requires exactly one type argument")
				return nil
			}
			if elemType := c.resolveTypeAnnotation(node.TypeArguments[0]); elemType != nil {
				return types.NewReadonlyType(&types.ArrayType{ElementType: elemType})
			}
			return nil

		case "Promise":
			if len(node.TypeArguments) != 1 {
				c.addError(node, "Promise requires exactly one type argument")
				return nil
			}
			valueType := c.resolveTypeAnnotation(node.TypeArguments[0])
			if valueType == nil {
				return nil // Error already reported
			}
			// Use the global PromiseGeneric type to create a proper InstantiatedType
			if types.PromiseGeneric != nil {
				return types.NewInstantiatedType(types.PromiseGeneric, []types.Type{valueType})
			}
			// Fallback: simple ObjectType with Promise-like structure
			promiseType := types.NewObjectType()
			promiseType.WithProperty("then", types.Any)
			promiseType.WithProperty("catch", types.Any)
			return promiseType

		default:
			// Check if this is a forward reference to the current generic class
			var baseType types.Type
			var exists bool

			if node.Qualifier != nil {
				// N.G<T> / z.infer<T>: look G up in the namespace or module.
				baseType = c.resolveEnumMemberTypeExpression(&parser.MemberExpression{
					Token:    node.Token,
					Object:   node.Qualifier,
					Property: node.Name,
				})
				if baseType == nil {
					for _, arg := range node.TypeArguments {
						c.resolveTypeAnnotation(arg)
					}
					return nil
				}
				exists = true
			} else if c.currentForwardRef != nil && node.Name.Value == c.currentForwardRef.ClassName {
				debugPrintf("// [Checker resolveTypeAnno GenericTypeRef] Resolved '%s' as forward reference to current generic class\n", node.Name.Value)
				baseType = c.currentForwardRef
				exists = true
			} else {
				// Check if this is a recursive reference to the type alias being defined
				if c.resolvingTypeAliases[node.Name.Value] {
					debugPrintf("// [Checker resolveTypeAnno GenericTypeRef] Detected recursive generic reference to '%s', creating placeholder\n", node.Name.Value)
					// Create a placeholder for the recursive reference
					// We'll use a special GenericForwardReference type that includes type arguments
					// Keep the arguments: they mention the alias's own type
					// parameters (`DeepReadonly<T[K]>`), which instantiation
					// substitutes, so the reference can be expanded later (#613).
					// Speculative: an argument naming an `infer` variable that
					// isn't in scope here would report TS2304; keep the
					// placeholders then.
					args := make([]types.Type, len(node.TypeArguments))
					if !c.speculate(func() {
						for i, arg := range node.TypeArguments {
							args[i] = c.resolveTypeAnnotation(arg)
						}
					}) {
						args = make([]types.Type, len(node.TypeArguments))
					}
					return &types.GenericTypeAliasForwardReference{
						AliasName:     node.Name.Value,
						Scope:         c.env.ResolveType,
						TypeArguments: args,
					}
				}

				debugPrintf("// [Checker resolveTypeAnno GenericTypeRef] Resolving generic type '%s' in environment. Current resolving: %v\n", node.Name.Value, c.resolvingTypeAliases)

				// Check if this is a user-defined generic type
				baseType, exists = c.env.ResolveType(node.Name.Value)
				if !exists && c.resolvePendingAlias(node.Name.Value) {
					baseType, exists = c.env.ResolveType(node.Name.Value)
				}
			}

			if !exists {
				// A generic declared later in the program (or imported, or from a
				// lib Paserati does not model) is only a forward reference; a name
				// nothing declares is TS2304.
				if !c.mayBeDeclaredGeneric(node.Name.Value) {
					c.addCannotFindTypeNameError(node.Name, c.env, node.Name.Value)
					for _, arg := range node.TypeArguments {
						c.resolveTypeAnnotation(arg)
					}
					return nil
				}
				debugPrintf("// [Checker resolveTypeAnno GenericTypeRef] Creating forward reference for unknown type '%s'\n", node.Name.Value)
				// Keep the arguments when they resolve, so the reference can be
				// instantiated once the generic is declared.
				args := make([]types.Type, len(node.TypeArguments))
				if !c.speculate(func() {
					for i, arg := range node.TypeArguments {
						args[i] = c.resolveTypeAnnotation(arg)
					}
				}) {
					args = make([]types.Type, len(node.TypeArguments))
				}
				return &types.GenericTypeAliasForwardReference{
					AliasName:     node.Name.Value,
					Scope:         c.env.ResolveType,
					TypeArguments: args,
				}
			}

			// Check if it's a ForwardReferenceType (self-reference during class definition)
			if _, ok := baseType.(*types.ForwardReferenceType); ok {
				// Create a parameterized forward reference that preserves type arguments
				if checkerDebug {
					debugPrintf("// [Checker resolveTypeAnno GenericTypeRef] Creating ParameterizedForwardReferenceType for '%s' with %d type args\n", node.Name.Value, len(node.TypeArguments))
				}

				// Resolve type arguments
				typeArgs := make([]types.Type, len(node.TypeArguments))
				for i, argExpr := range node.TypeArguments {
					argType := c.resolveTypeAnnotation(argExpr)
					if argType == nil {
						return nil // Error already reported
					}
					typeArgs[i] = argType
				}

				// Return a parameterized forward reference that preserves the type arguments
				return &types.ParameterizedForwardReferenceType{
					ClassName:     node.Name.Value,
					TypeArguments: typeArgs,
				}
			}

			// Check if it's a GenericType
			if genericType, ok := baseType.(*types.GenericType); ok {
				// Validate type argument count
				// Check argument count - allow fewer arguments if defaults are available
				minRequired := 0
				for _, param := range genericType.TypeParameters {
					if param.Default == nil {
						minRequired++
					} else {
						break // Once we hit a parameter with a default, all following must have defaults
					}
				}

				if len(node.TypeArguments) < minRequired || len(node.TypeArguments) > len(genericType.TypeParameters) {
					if minRequired == len(genericType.TypeParameters) {
						c.addError(node, fmt.Sprintf("Generic type '%s' expects %d type arguments, got %d",
							node.Name.Value, len(genericType.TypeParameters), len(node.TypeArguments)))
					} else {
						c.addError(node, fmt.Sprintf("Generic type '%s' expects %d-%d type arguments, got %d",
							node.Name.Value, minRequired, len(genericType.TypeParameters), len(node.TypeArguments)))
					}
					return nil
				}

				// Resolve type arguments
				typeArgs := make([]types.Type, len(node.TypeArguments))
				for i, argExpr := range node.TypeArguments {
					argType := c.resolveTypeAnnotation(argExpr)
					if argType == nil {
						return nil // Error already reported
					}
					typeArgs[i] = argType
				}

				// Instantiate the generic type
				c.typeRefNode = node
				return c.instantiateGenericType(genericType, typeArgs, node.TypeArguments)
			} else if baseType == types.Any {
				// Special case: if baseType is Any (unresolved import) and we have type arguments,
				// allow it and create a placeholder generic instantiation
				// This is common for type-only imports where we don't have the full type information
				if checkerDebug {
					debugPrintf("// [Checker resolveTypeAnno GenericTypeRef] Allowing generic syntax on unresolved type '%s' with %d type args\n", node.Name.Value, len(node.TypeArguments))
				}

				// Resolve type arguments even though we don't know the base type
				typeArgs := make([]types.Type, len(node.TypeArguments))
				for i, argExpr := range node.TypeArguments {
					argType := c.resolveTypeAnnotation(argExpr)
					if argType == nil {
						return nil // Error already reported
					}
					typeArgs[i] = argType
				}

				// Return a generic forward reference that preserves the type arguments
				// This allows interface inheritance and other generic type usage to work
				return &types.GenericTypeAliasForwardReference{
					AliasName:     node.Name.Value,
					Scope:         c.env.ResolveType,
					TypeArguments: typeArgs,
				}
			} else {
				c.addErrorWithCode(node, errors.TS2315, fmt.Sprintf("Type '%s' is not generic.", node.Name.Value))
				return nil
			}
		}

	// --- NEW: Handle ConstructorTypeExpression ---
	case *parser.ConstructorTypeExpression:
		return c.resolveConstructorTypeSignature(node)

	case *parser.ReadonlyTypeExpression:
		inner := c.resolveTypeAnnotation(node.Type)
		switch inner.(type) {
		case *types.ArrayType, *types.TupleType:
			return types.NewReadonlyType(inner)
		}
		return inner

	case *parser.KeyofTypeExpression:
		return c.resolveKeyofTypeExpression(node)

	case *parser.TypeofTypeExpression:
		return c.resolveTypeofTypeExpression(node)

	case *parser.ImportTypeExpression:
		// A module is not a type: `typeof import("x")` is.
		if c.importTypeNamespace(node) != nil {
			c.addErrorWithCode(node, errors.TS1340, fmt.Sprintf("Module '%s' does not refer to a type, but is used as a type here. Did you mean 'typeof import('%s')'?", node.Specifier, node.Specifier))
		}
		return types.Any

	case *parser.InferTypeExpression:
		return c.resolveInferTypeExpression(node)

	case *parser.TypePredicateExpression:
		return c.resolveTypePredicateExpression(node)

	case *parser.MappedTypeExpression:
		return c.resolveMappedTypeExpression(node)

	case *parser.IndexedAccessTypeExpression:
		return c.resolveIndexedAccessTypeExpression(node)

	case *parser.ConditionalTypeExpression:
		return c.resolveConditionalTypeExpression(node)

	case *parser.TemplateLiteralTypeExpression:
		return c.resolveTemplateLiteralTypeExpression(node)

	case *parser.MemberExpression:
		resolved := c.resolveEnumMemberTypeExpression(node)
		if g, ok := resolved.(*types.GenericType); ok {
			// `z.Base` without type arguments: the defaults (TS2314 when a
			// parameter has none).
			return c.instantiateGenericType(g, nil, nil)
		}
		return resolved

	default:
		// If we get here, the parser created a node type that resolveTypeAnnotation doesn't handle yet.
		c.addError(node, fmt.Sprintf("unsupported type annotation node: %T", node))
		return nil // Indicate error
	}
}

// --- NEW: Helper to resolve FunctionTypeExpression nodes ---
func (c *Checker) resolveFunctionTypeSignature(node *parser.FunctionTypeExpression) types.Type {
	// If this is a generic function type, handle type parameters
	if len(node.TypeParameters) > 0 {
		return c.resolveGenericFunctionType(node)
	}

	// Non-generic function type - use existing logic
	paramTypes := []types.Type{}
	for _, paramNode := range node.Parameters {
		paramType := c.resolveTypeAnnotation(paramNode)
		if paramType == nil {
			// Error should have been added by resolveTypeAnnotation
			return nil // Indicate error by returning nil
		}
		paramTypes = append(paramTypes, paramType)
	}

	// Handle rest parameter if present
	var restParameterType types.Type
	if node.RestParameter != nil {
		resolvedRestType := c.resolveTypeAnnotation(node.RestParameter)
		if resolvedRestType != nil {
			// Validate that rest parameter type is an array or tuple type
			if !isValidRestParameterType(resolvedRestType) {
				c.addErrorWithCode(node.RestParameter, errors.TS2370, "A rest parameter must be of an array type.")
				resolvedRestType = &types.ArrayType{ElementType: types.Any}
			}
		} else {
			// Default to any[] if resolution failed
			resolvedRestType = &types.ArrayType{ElementType: types.Any}
		}
		restParameterType = resolvedRestType
	}

	var returnType types.Type = types.Any
	if node.ReturnType != nil {
		resolvedReturnType := c.resolveTypeAnnotation(node.ReturnType)
		if resolvedReturnType == nil {
			// Error should have been added by resolveTypeAnnotation
			return nil // Indicate error by returning nil
		}
		returnType = resolvedReturnType
	}

	// Create signature
	sig := &types.Signature{
		ParameterNames:    functionTypeParameterNameStrings(node.Parameters),
		ParameterTypes:    paramTypes,
		ReturnType:        returnType,
		OptionalParams:    node.OptionalParams,
		IsVariadic:        node.RestParameter != nil,
		RestParameterType: restParameterType,
		StrictVariance:    !node.IsMethodSignature,
	}
	if node.ThisType != nil {
		sig.ThisType = c.resolveTypeAnnotation(node.ThisType)
	}

	// Create a unified ObjectType with call signature
	return types.NewFunctionType(sig)
}

// resolveGenericFunctionType handles generic function types like <T>(x: T) => T
func (c *Checker) resolveGenericFunctionType(node *parser.FunctionTypeExpression) types.Type {
	// Create the type parameter environment first so constraints/defaults can
	// reference any sibling type parameter, including later declarations.
	typeParamEnv := NewEnclosedEnvironment(c.env)
	typeParams := make([]*types.TypeParameter, len(node.TypeParameters))
	for i, paramNode := range node.TypeParameters {
		typeParam := &types.TypeParameter{
			Name:       paramNode.Name.Value,
			Constraint: types.Any, // Default constraint
			Index:      i,
		}
		typeParams[i] = typeParam
		typeParamEnv.DefineTypeParameter(typeParam.Name, typeParam)
		paramNode.SetComputedType(&types.TypeParameterType{Parameter: typeParam})
	}

	originalEnv := c.env
	c.env = typeParamEnv
	for i, paramNode := range node.TypeParameters {
		typeParam := typeParams[i]

		// Handle constraint if present
		if paramNode.Constraint != nil {
			constraint := c.resolveTypeAnnotation(paramNode.Constraint)
			if constraint != nil {
				typeParam.Constraint = constraint
			}
		}
	}

	// Resolve parameter types and return type in the new environment
	paramTypes := []types.Type{}
	for _, paramNode := range node.Parameters {
		paramType := c.resolveTypeAnnotation(paramNode)
		if paramType == nil {
			c.env = originalEnv // Restore environment
			return nil
		}
		paramTypes = append(paramTypes, paramType)
	}

	// Handle rest parameter if present
	var restParameterType types.Type
	if node.RestParameter != nil {
		resolvedRestType := c.resolveTypeAnnotation(node.RestParameter)
		if resolvedRestType != nil {
			if !isValidRestParameterType(resolvedRestType) {
				c.addErrorWithCode(node.RestParameter, errors.TS2370, "A rest parameter must be of an array type.")
				resolvedRestType = &types.ArrayType{ElementType: types.Any}
			}
		} else {
			resolvedRestType = &types.ArrayType{ElementType: types.Any}
		}
		restParameterType = resolvedRestType
	}

	var returnType types.Type = types.Any
	if node.ReturnType != nil {
		resolvedReturnType := c.resolveTypeAnnotation(node.ReturnType)
		if resolvedReturnType == nil {
			c.env = originalEnv // Restore environment
			return nil
		}
		returnType = resolvedReturnType
	}

	// Restore original environment
	c.env = originalEnv

	// Create the function signature
	sig := &types.Signature{
		TypeParameters:    typeParams,
		ParameterNames:    functionTypeParameterNameStrings(node.Parameters),
		ParameterTypes:    paramTypes,
		ReturnType:        returnType,
		OptionalParams:    node.OptionalParams,
		IsVariadic:        node.RestParameter != nil,
		RestParameterType: restParameterType,
		StrictVariance:    !node.IsMethodSignature,
	}
	if node.ThisType != nil {
		originalEnvForThis := c.env
		c.env = typeParamEnv
		sig.ThisType = c.resolveTypeAnnotation(node.ThisType)
		c.env = originalEnvForThis
	}

	// Create the function type
	functionType := types.NewFunctionType(sig)

	// Wrap in a GenericType
	genericType := &types.GenericType{
		Name:           "Function", // Generic function type
		TypeParameters: typeParams,
		Body:           functionType,
	}

	return genericType
}

// extractCallSignaturesFromType returns the call signatures of a function type
// used as a call-signature member. Such signatures are not methods, so they
// are compared contravariantly under strictFunctionTypes, and a generic
// function's type parameters are recorded on its signatures.
func (c *Checker) extractCallSignaturesFromType(typ types.Type) []*types.Signature {
	switch t := typ.(type) {
	case *types.ObjectType:
		if len(t.CallSigs()) > 0 {
			return strictSignatureCopies(t.CallSigs(), nil)
		}
	case *types.GenericType:
		if bodyObj, ok := t.Body.(*types.ObjectType); ok && len(bodyObj.CallSigs()) > 0 {
			return strictSignatureCopies(bodyObj.CallSigs(), t.TypeParameters)
		}
	}
	return nil
}

func (c *Checker) extractConstructSignaturesFromType(typ types.Type) []*types.Signature {
	switch t := typ.(type) {
	case *types.ObjectType:
		if len(t.ConstructSigs()) > 0 {
			return strictSignatureCopies(t.ConstructSigs(), nil)
		}
	case *types.GenericType:
		if bodyObj, ok := t.Body.(*types.ObjectType); ok && len(bodyObj.ConstructSigs()) > 0 {
			return strictSignatureCopies(bodyObj.ConstructSigs(), t.TypeParameters)
		}
	}
	return nil
}

// strictSignatureCopies copies signatures marking them as non-method (strict
// variance) and, when typeParams is given, records them as the signatures' own
// type parameters.
func strictSignatureCopies(sigs []*types.Signature, typeParams []*types.TypeParameter) []*types.Signature {
	out := make([]*types.Signature, len(sigs))
	for i, sig := range sigs {
		cp := *sig
		cp.StrictVariance = true
		if len(typeParams) > 0 && len(cp.TypeParameters) == 0 {
			cp.TypeParameters = append([]*types.TypeParameter(nil), typeParams...)
		}
		out[i] = &cp
	}
	return out
}

// --- NEW: Helper to resolve ObjectTypeExpression nodes ---
func (c *Checker) resolveObjectTypeSignature(node *parser.ObjectTypeExpression) types.Type {
	properties := make(map[string]types.Type)
	var propertyOrder []string
	propertyDocs := make(map[string]string)
	optionalProperties := make(map[string]bool)
	readOnlyProperties := make(map[string]bool)
	var callSignatures []*types.Signature
	var constructSignatures []*types.Signature
	var indexSignatures []*types.IndexSignature

	for _, prop := range node.Properties {
		if prop.IsCallSignature {
			if prop.Type != nil {
				propType := c.resolveTypeAnnotation(prop.Type)
				if sigs := c.extractCallSignaturesFromType(propType); len(sigs) > 0 {
					callSignatures = append(callSignatures, sigs...)
					continue
				}
			}

			// Handle call signature like (param: type): returnType
			var paramTypes []types.Type
			for _, paramNode := range prop.Parameters {
				paramType := c.resolveTypeAnnotation(paramNode)
				if paramType == nil {
					paramType = types.Any
				}
				paramTypes = append(paramTypes, paramType)
			}

			returnType := c.resolveTypeAnnotation(prop.ReturnType)
			if returnType == nil {
				returnType = types.Any
			}

			// Create a signature
			sig := &types.Signature{
				ParameterTypes: paramTypes,
				ReturnType:     returnType,
				StrictVariance: true,
				// Note: Object type call signatures don't track optional parameters for now
			}

			callSignatures = append(callSignatures, sig)
		} else if prop.IsConstructSignature {
			if prop.Type != nil {
				propType := c.resolveTypeAnnotation(prop.Type)
				if sigs := c.extractConstructSignaturesFromType(propType); len(sigs) > 0 {
					constructSignatures = append(constructSignatures, sigs...)
				}
			}
		} else if prop.IsIndexSignature {
			// Handle index signature like [key: string]: Type
			// For now, we'll add them to IndexSignatures field of ObjectType
			keyType := c.resolveTypeAnnotation(prop.KeyType)
			if keyType == nil {
				keyType = types.Any
			}
			valueType := c.resolveTypeAnnotation(prop.ValueType)
			if valueType == nil {
				valueType = types.Any
			}

			indexSig := &types.IndexSignature{
				KeyType:   keyType,
				ValueType: valueType,
				Readonly:  prop.Readonly,
			}

			// We'll add this to the ObjectType's IndexSignatures field
			if checkerDebug {
				debugPrintf("// [Checker ObjectType] Index signature [%s: %s]: %s\n",
					prop.KeyName.Value, keyType.String(), valueType.String())
			}

			// Add to our collected index signatures
			c.reportDuplicateIndexSignature(prop.KeyName, indexSignatures, keyType)
			indexSignatures = append(indexSignatures, indexSig)

		} else if prop.IsComputedProperty {
			// This is a computed property: [expr]: Type
			if prop.ComputedName != nil {
				c.visit(prop.ComputedName) // check for undefined identifiers in computed key
			}
			propType := c.resolveTypeAnnotation(prop.Type)
			if propType == nil {
				propType = types.Any
			}

			// Try to extract a constant property name from the computed expression
			computedName := c.extractConstantPropertyName(prop.ComputedName)
			if computedName != "" {
				// We can resolve this to a concrete property name
				types.SetOrdered(properties, &propertyOrder, computedName, propType)
				if prop.Optional {
					optionalProperties[computedName] = true
				}
				if checkerDebug {
					debugPrintf("// [Checker ObjectType] Computed property '%s': %s\n",
						computedName, propType.String())
				}
			} else {
				// Dynamic computed property - treat as index signature for now
				debugPrintf("// [Checker ObjectType] Dynamic computed property, treating as index signature\n")
				indexSignature := &types.IndexSignature{
					KeyType:   types.String, // Assume string keys for now
					Synthetic: true,
					ValueType: propType,
				}
				indexSignatures = append(indexSignatures, indexSignature)
			}
		} else if prop.Name != nil {
			// Regular property or method
			propType := c.resolveTypeAnnotation(prop.Type)
			if propType == nil {
				propType = types.Any
			}
			if _, exists := properties[prop.Name.Value]; exists {
				if optionalProperties[prop.Name.Value] != prop.Optional {
					c.addError(prop.Name, fmt.Sprintf("All declarations of '%s' must have identical optionality.", prop.Name.Value))
				}
			}
			// Method overloads: `m(x: number): number; m(s: string): string;`
			// accumulate into one callable type with several call signatures.
			if prev, exists := properties[prop.Name.Value]; exists {
				prevSigs, prevOk := overloadableCallSignatures(prev)
				curSigs, curOk := overloadableCallSignatures(propType)
				if prevOk && curOk {
					merged := &types.ObjectType{}
					merged.Materialize()
					merged.CallSignatures = append(merged.CallSigs(), prevSigs...)
					merged.Materialize()
					merged.CallSignatures = append(merged.CallSigs(), curSigs...)
					propType = merged
				}
			}
			types.SetOrdered(properties, &propertyOrder, prop.Name.Value, propType)
			if prop.Doc != "" {
				propertyDocs[prop.Name.Value] = prop.Doc
			}
			if prop.Optional {
				optionalProperties[prop.Name.Value] = true
			}
			if prop.Readonly {
				readOnlyProperties[prop.Name.Value] = true
			}
			if checkerDebug {
				debugPrintf("// [Checker ObjectType] Property '%s'%s: %s\n",
					prop.Name.Value,
					func() string {
						if prop.Optional {
							return "?"
						} else {
							return ""
						}
					}(),
					propType.String())
			}
		}
		// Skip properties with nil names that aren't call signatures
	}

	// Create a unified ObjectType
	objectType := &types.ObjectType{
		Properties:          properties,
		PropertyOrder:       propertyOrder,
		PropertyDocs:        propertyDocs,
		OptionalProperties:  optionalProperties,
		ReadOnlyProperties:  readOnlyProperties,
		CallSignatures:      callSignatures,
		ConstructSignatures: constructSignatures,
		IndexSignatures:     indexSignatures,
	}

	// Properties of a type literal are constrained by its index signatures.
	if len(indexSignatures) > 0 {
		for _, prop := range node.Properties {
			if prop.IsCallSignature || prop.IsConstructSignature || prop.IsIndexSignature {
				continue
			}
			if prop.IsComputedProperty && prop.ComputedName != nil {
				if name := c.extractConstantPropertyName(prop.ComputedName); name != "" {
					c.checkPropertyAgainstIndexSignatures(nil, prop.ComputedName, name, properties[name], indexSignatures)
				}
				continue
			}
			if prop.Name == nil {
				continue
			}
			c.checkPropertyAgainstIndexSignatures(nil, prop.Name, prop.Name.Value, properties[prop.Name.Value], indexSignatures)
		}
	}

	// If it's a pure callable object with no properties and exactly one signature,
	// we can make it a pure function type for better type display
	if len(properties) == 0 && len(callSignatures) == 1 {
		// Create a fresh object to ensure we get a "pure function"
		// (the IsPureFunction helper method will return true)
		return types.NewFunctionType(callSignatures[0])
	}

	return objectType
}

// --- NEW: Helper to resolve ConstructorTypeExpression nodes ---
func (c *Checker) resolveConstructorTypeSignature(node *parser.ConstructorTypeExpression) types.Type {
	originalEnv := c.env
	var sigTypeParams []*types.TypeParameter
	if len(node.TypeParameters) > 0 {
		typeParamEnv := NewEnclosedEnvironment(c.env)
		typeParams := make([]*types.TypeParameter, len(node.TypeParameters))
		sigTypeParams = typeParams
		for i, paramNode := range node.TypeParameters {
			typeParam := &types.TypeParameter{
				Name:       paramNode.Name.Value,
				Constraint: types.Any,
				Index:      i,
			}
			typeParams[i] = typeParam
			typeParamEnv.DefineTypeParameter(typeParam.Name, typeParam)
			paramNode.SetComputedType(&types.TypeParameterType{Parameter: typeParam})
		}

		c.env = typeParamEnv
		for i, paramNode := range node.TypeParameters {
			if paramNode.Constraint != nil {
				c.checkTypeRefDefined(paramNode.Constraint)
				if constraint := c.resolveTypeAnnotation(paramNode.Constraint); constraint != nil {
					typeParams[i].Constraint = constraint
				}
			}
			if paramNode.DefaultType != nil {
				if defaultType := c.resolveTypeAnnotation(paramNode.DefaultType); defaultType != nil {
					typeParams[i].Default = defaultType
				}
			}
		}
		defer func() { c.env = originalEnv }()
	}

	paramTypes := []types.Type{}
	for _, paramNode := range node.Parameters {
		paramType := c.resolveTypeAnnotation(paramNode)
		if paramType == nil {
			// Error should have been added by resolveTypeAnnotation
			return nil // Indicate error by returning nil
		}
		paramTypes = append(paramTypes, paramType)
	}

	var restParameterType types.Type
	if node.RestParameter != nil {
		resolvedRestType := c.resolveTypeAnnotation(node.RestParameter)
		if resolvedRestType != nil {
			if !isValidRestParameterType(resolvedRestType) {
				c.addErrorWithCode(node.RestParameter, errors.TS2370, "A rest parameter must be of an array type.")
				resolvedRestType = &types.ArrayType{ElementType: types.Any}
			}
		} else {
			resolvedRestType = &types.ArrayType{ElementType: types.Any}
		}
		restParameterType = resolvedRestType
	}

	var constructedType types.Type = types.Any
	if node.ReturnType != nil {
		resolvedReturnType := c.resolveTypeAnnotation(node.ReturnType)
		if resolvedReturnType == nil {
			// Error should have been added by resolveTypeAnnotation
			return nil // Indicate error by returning nil
		}
		constructedType = resolvedReturnType
	}

	// Create signature
	sig := &types.Signature{
		TypeParameters:    sigTypeParams,
		ParameterTypes:    paramTypes,
		ReturnType:        constructedType,
		OptionalParams:    node.OptionalParams,
		IsVariadic:        node.RestParameter != nil,
		RestParameterType: restParameterType,
		StrictVariance:    true,
		// Note: Constructor type expressions don't track optional parameters
	}

	// Create a unified ObjectType with constructor signature
	return types.NewConstructorType(sig)
}

// Resolves parameter and return type annotations within the given environment.
// Returns a types.Signature for the unified type system
func (c *Checker) resolveFunctionLiteralSignature(node *parser.FunctionLiteral, env *Environment) *types.Signature {
	paramTypes := []types.Type{}
	var optionalParams []bool

	if len(node.TypeParameters) > 0 {
		typeParamEnv := NewEnclosedEnvironment(env)
		typeParams := make([]*types.TypeParameter, len(node.TypeParameters))
		for i, paramNode := range node.TypeParameters {
			typeParam := &types.TypeParameter{
				Name:       paramNode.Name.Value,
				Constraint: types.Any,
				Index:      i,
			}
			typeParams[i] = typeParam
			typeParamEnv.DefineTypeParameter(typeParam.Name, typeParam)
			paramNode.SetComputedType(&types.TypeParameterType{Parameter: typeParam})
		}

		originalEnv := c.env
		c.env = typeParamEnv
		for i, paramNode := range node.TypeParameters {
			if paramNode.Constraint != nil {
				if constraint := c.resolveTypeAnnotation(paramNode.Constraint); constraint != nil {
					typeParams[i].Constraint = constraint
				}
			}
			if paramNode.DefaultType != nil {
				if defaultType := c.resolveTypeAnnotation(paramNode.DefaultType); defaultType != nil {
					typeParams[i].Default = defaultType
				}
			}
		}
		c.env = originalEnv
		env = typeParamEnv
	}

	// Create a temporary environment that will progressively accumulate parameters
	// This allows later parameters to reference earlier ones in their default values
	tempEnv := NewEnclosedEnvironment(env) // Create child environment

	for _, paramNode := range node.Parameters {
		var resolvedParamType types.Type
		if paramNode.TypeAnnotation != nil {
			// Temporarily use the provided environment for resolving the annotation
			originalEnv := c.env
			c.env = env
			resolvedParamType = c.resolveTypeAnnotation(paramNode.TypeAnnotation)
			c.env = originalEnv // Restore original environment
		}

		// NEW: If no type annotation but has default value, infer type from default value
		if resolvedParamType == nil && paramNode.DefaultValue != nil {
			// Use the temporary environment that includes previously defined parameters
			originalEnv := c.env
			c.env = tempEnv                 // Use progressive environment that includes earlier parameters
			c.visit(paramNode.DefaultValue) // This will set the computed type
			c.env = originalEnv             // Restore original environment

			defaultValueType := paramNode.DefaultValue.GetComputedType()
			if defaultValueType != nil {
				// Widen literal types for parameter inference (like let/const inference)
				resolvedParamType = types.GetWidenedType(defaultValueType)
				paramName := "<destructuring>"
				if paramNode.Name != nil {
					paramName = paramNode.Name.Value
				}
				if checkerDebug {
					debugPrintf("// [Checker resolveFuncLitType] Inferred parameter '%s' type from default value: %s -> %s\n",
						paramName, defaultValueType.String(), resolvedParamType.String())
				}
			}
		}

		if resolvedParamType == nil {
			resolvedParamType = types.Any // Default to Any if no annotation or resolution failed
		}
		paramTypes = append(paramTypes, resolvedParamType)

		// Add this parameter to the temporary environment BEFORE checking its default value
		// This way, the next parameter's default value can reference this parameter
		// Skip 'this' parameters as they don't have names and don't go into the scope
		// Skip destructuring parameters as they don't have a single name (bindings are added during destructuring)
		if !paramNode.IsThis && !paramNode.IsDestructuring {
			tempEnv.Define(paramNode.Name.Value, resolvedParamType, false) // false = not const
		}

		// Validate default value if present (skip if we already visited it for inference)
		if paramNode.DefaultValue != nil && paramNode.TypeAnnotation != nil {
			// Only validate if we had an explicit annotation (inference case already visited above)
			// Use the temporary environment that includes previously defined parameters
			originalEnv := c.env
			c.env = tempEnv                 // Use progressive environment that includes earlier parameters
			c.visit(paramNode.DefaultValue) // This will set the computed type
			c.env = originalEnv             // Restore original environment

			defaultValueType := paramNode.DefaultValue.GetComputedType()
			if defaultValueType != nil && !c.assignableToFresh(paramNode.DefaultValue, defaultValueType, resolvedParamType) {
				var errNode parser.Node = paramNode.DefaultValue
				if paramNode.Name != nil {
					errNode = paramNode.Name
				}
				c.reportNotAssignable(errNode, paramNode.DefaultValue, defaultValueType, resolvedParamType, headAssign)
			}
		}

		// Parameter is optional if explicitly marked OR has a default value
		isOptional := paramNode.Optional || (paramNode.DefaultValue != nil)
		optionalParams = append(optionalParams, isOptional)
	}

	// Handle rest parameter if present
	var restParameterType types.Type
	if node.RestParameter != nil {
		var resolvedRestType types.Type
		if node.RestParameter.TypeAnnotation != nil {
			originalEnv := c.env
			c.env = env
			resolvedRestType = c.resolveTypeAnnotation(node.RestParameter.TypeAnnotation)
			c.env = originalEnv

			// Rest parameter type should be an array or tuple type
			if resolvedRestType != nil {
				if !isValidRestParameterType(resolvedRestType) {
					c.addErrorWithCode(node.RestParameter.TypeAnnotation, errors.TS2370, "A rest parameter must be of an array type.")
					resolvedRestType = &types.ArrayType{ElementType: types.Any}
				}
			}
		}

		if resolvedRestType == nil {
			// Default to any[] if no annotation
			resolvedRestType = &types.ArrayType{ElementType: types.Any}
		}

		restParameterType = resolvedRestType
		node.RestParameter.ComputedType = restParameterType
	}

	var resolvedReturnType types.Type         // Keep as interface type
	var resolvedTypeFromAnnotation types.Type // Temp variable, also interface type

	funcNameForLog := "<anonymous_resolve>"
	if node.Name != nil {
		funcNameForLog = node.Name.Value
	}

	if node.ReturnTypeAnnotation != nil {
		originalEnv := c.env
		c.env = env
		debugPrintf("// [Checker resolveFuncLitType] Resolving return annotation (%T) for func '%s'\n", node.ReturnTypeAnnotation, funcNameForLog)
		// Assign to the temporary variable first
		resolvedTypeFromAnnotation = c.resolveTypeAnnotation(node.ReturnTypeAnnotation)
		debugPrintf("// [Checker resolveFuncLitType] resolveTypeAnnotation returned: Ptr=%p, Type=%T, Value=%#v for func '%s'\n", resolvedTypeFromAnnotation, resolvedTypeFromAnnotation, resolvedTypeFromAnnotation, funcNameForLog)
		// DO NOT assign to resolvedReturnType here yet
		c.env = originalEnv
	} else {
		debugPrintf("// [Checker resolveFuncLitType] No return type annotation for func '%s'\n", funcNameForLog)
		resolvedTypeFromAnnotation = nil // Ensure temp var is nil if no annotation
	}

	// Now, assign the result from the temporary variable to the main one
	resolvedReturnType = resolvedTypeFromAnnotation
	debugPrintf("// [Checker resolveFuncLitType] Assigned final resolvedReturnType: Ptr=%p, Type=%T, Value=%#v for func '%s'\n", resolvedReturnType, resolvedReturnType, resolvedReturnType, funcNameForLog) // ADDED LOG

	// The final check should now behave correctly
	if resolvedReturnType == nil {
		debugPrintf("// [Checker resolveFuncLitType] Final check: resolvedReturnType is nil for func '%s'\n", funcNameForLog)
		// resolvedReturnType = nil // It's already nil, no need to re-assign
	} else {
		debugPrintf("// [Checker resolveFuncLitType] Final check: resolvedReturnType is NOT nil (Ptr=%p) for func '%s'\n", resolvedReturnType, funcNameForLog)
	}

	// Return a unified Signature
	sig := &types.Signature{
		ParameterNames:    parameterNameStrings(node.Parameters),
		ParameterTypes:    paramTypes,
		ReturnType:        resolvedReturnType, // Use the value assigned outside the if
		OptionalParams:    optionalParams,
		IsVariadic:        node.RestParameter != nil,
		RestParameterType: restParameterType,
	}
	dropThisParam(sig, node.Parameters)
	return sig
}

func functionTypeParameterNameStrings(params []parser.Expression) []string {
	return nil
}

// instantiateGenericType creates a concrete type by substituting type arguments
// into a GenericType's body type
func (c *Checker) instantiateGenericType(genericType *types.GenericType, typeArgs []types.Type, typeArgNodes []parser.Expression) types.Type {
	debugPrintf("// [InstantiateGeneric] === INSTANTIATING GENERIC TYPE ===\n")
	debugPrintf("// [InstantiateGeneric] Generic name: %s\n", genericType.Name)
	if checkerDebug {
		debugPrintf("// [InstantiateGeneric] Body type: %s (Go type: %T)\n", genericType.Body.String(), genericType.Body)
	}
	// Handle default type parameters when fewer arguments are provided
	finalTypeArgs := make([]types.Type, len(genericType.TypeParameters))

	// Fill in provided type arguments
	for i := 0; i < len(typeArgs) && i < len(genericType.TypeParameters); i++ {
		finalTypeArgs[i] = typeArgs[i]
	}

	// Fill in defaults for missing type arguments
	for i := len(typeArgs); i < len(genericType.TypeParameters); i++ {
		typeParam := genericType.TypeParameters[i]
		if typeParam.Default != nil {
			// A default may name earlier parameters (`B = A`): substitute
			// the arguments chosen so far (#616).
			earlier := make(map[string]types.Type, i)
			for j := 0; j < i; j++ {
				earlier[genericType.TypeParameters[j].Name] = finalTypeArgs[j]
			}
			finalTypeArgs[i] = c.substituteTypes(typeParam.Default, earlier)
			if checkerDebug {
				debugPrintf("// [Checker] Using default type '%s' for parameter '%s'\n",
					typeParam.Default.String(), typeParam.Name)
			}
		} else {
			// Error: no type argument provided and no default (TS2314 / TS2707)
			minArgs := len(genericType.TypeParameters)
			for minArgs > 0 && genericType.TypeParameters[minArgs-1].Default != nil {
				minArgs--
			}
			if minArgs < len(genericType.TypeParameters) {
				c.reportGenericArity(errors.TS2707, fmt.Sprintf("Generic type '%s' requires between %d and %d type arguments.", genericType.Name, minArgs, len(genericType.TypeParameters)))
			} else {
				c.reportGenericArity(errors.TS2314, fmt.Sprintf("Generic type '%s' requires %d type argument(s).", genericType.Name, minArgs))
			}
			return types.Any
		}
	}

	if checkerDebug {
		var typeStrs []string
		for _, t := range finalTypeArgs {
			typeStrs = append(typeStrs, t.String())
		}
		debugPrintf("// [Checker] Instantiating generic type '%s' with args [%s]\n",
			genericType.Name, strings.Join(typeStrs, ", "))
	}

	// Create substitution map from type parameters to concrete types. Use it for
	// constraint validation too, since constraints may reference sibling type
	// parameters, e.g. <T extends U, U extends Array<number>>.
	substitution := make(map[string]types.Type)
	for i, typeParam := range genericType.TypeParameters {
		substitution[typeParam.Name] = finalTypeArgs[i]
	}

	// Validate constraints before instantiation (not in a library file under
	// skipLibCheck)
	for i, typeParam := range genericType.TypeParameters {
		if typeParam.Constraint != nil && !c.skipValidation() {
			argType := c.resolveTypeofTypeIfNeeded(finalTypeArgs[i])
			if _, deferred := argType.(*types.TypeofType); deferred {
				// `typeof X` before X has a type (a hoisted alias): there is
				// nothing to check yet.
				continue
			}
			// A type parameter argument satisfies the constraint through its
			// own: PassthroughType<UnknownKeys> inside a generic whose
			// UnknownKeys extends UnknownKeysParam.
			for depth := 0; depth < 8; depth++ {
				tp, ok := argType.(*types.TypeParameterType)
				if !ok || tp.Parameter == nil || tp.Parameter.Constraint == nil {
					break
				}
				argType = tp.Parameter.Constraint
			}
			constraintType := c.substituteTypes(typeParam.Constraint, substitution)

			if checkerDebug {
				debugPrintf("// [Checker] Checking constraint: %s extends %s\n",
					argType.String(), constraintType.String())
			}

			if reachesPending(argType, 0, nil) || reachesPending(constraintType, 0, nil) {
				// A class or interface not filled in yet is reachable (a
				// hoisted alias names it, or `interface BB extends AA<BB>`
				// names BB in its own heritage): its members are filled in
				// place later, so the check runs once all declarations are
				// done.
				var argNode parser.Node
				if i < len(typeArgNodes) && typeArgNodes[i] != nil {
					argNode = typeArgNodes[i]
				} else if c.typeRefNode != nil {
					argNode = c.typeRefNode
				}
				arg, constraint := argType, constraintType
				c.deferRelationCheck(func() {
					if types.IsAssignable(arg, constraint) {
						return
					}
					msg := fmt.Sprintf("Type '%s' does not satisfy the constraint '%s'.", arg.String(), constraint.String())
					if argNode != nil {
						c.addErrorWithCode(argNode, errors.TS2344, msg)
					} else {
						c.addGenericError(msg)
					}
				})
				continue
			}

			// Check if the type argument satisfies the constraint
			if !types.IsAssignable(argType, constraintType) {
				// Create a more detailed error message with proper node position
				errorMsg := fmt.Sprintf("Type '%s' does not satisfy the constraint '%s'.",
					argType.String(), constraintType.String())

				// Use the specific type argument node for accurate error positioning
				if i < len(typeArgNodes) && typeArgNodes[i] != nil {
					c.addErrorWithCode(typeArgNodes[i], errors.TS2344, errorMsg)
				} else if c.typeRefNode != nil {
					c.addErrorWithCode(c.typeRefNode, errors.TS2344, errorMsg)
				} else {
					c.addGenericError(errorMsg)
				}
				return types.Any // Return any type to allow compilation to continue
			}
		}
	}

	// Perform type substitution on the body type. Once the generic's own
	// declaration is complete its body no longer changes, so the result is
	// cached per argument identities (the arguments are referenced, not
	// copied, so they may still be filling in).
	cacheable := c.instCache.complete[genericType]
	if !cacheable && isOwnTypeParameters(genericType, finalTypeArgs) {
		// `next?: Node<T>` inside `interface Node<T>`: the declared type
		// itself. Substituting would copy the body as declared so far,
		// without the members after this one.
		return genericType.Body
	}
	key := genericInstKey{generic: genericType, args: typeListKey(finalTypeArgs)}
	if cacheable {
		if cached, hit := c.instCache.inst[key]; hit {
			return cached
		}
	}
	instantiatedType := c.substituteTypes(genericType.Body, substitution)
	if obj, ok := instantiatedType.(*types.ObjectType); ok && obj != genericType.Body {
		if genericType.InstanceType != nil {
			// A class constructor: the instances it constructs instantiate
			// the class's instance generic (`extends ZodType<string, ...>`
			// instantiates the constructor).
			for _, sig := range obj.ConstructSigs() {
				if inst, ok := sig.ReturnType.(*types.ObjectType); ok && inst.Instantiates == nil &&
					len(inst.GenericArgs) == len(genericType.InstanceType.TypeParameters) {
					inst.Instantiates = genericType.InstanceType
				}
			}
		} else if obj.Instantiates == nil && len(obj.GenericArgs) == len(genericType.TypeParameters) {
			obj.Instantiates = genericType
		}
	}
	if cacheable {
		if len(c.instCache.inst) > 1<<18 {
			clear(c.instCache.inst)
		}
		c.instCache.inst[key] = instantiatedType
	}

	if checkerDebug {
		debugPrintf("// [Checker] Instantiated type: %s\n", instantiatedType.String())
	}
	return instantiatedType
}

// isOwnTypeParameters reports whether args are the generic's own type
// parameters, in order.
func isOwnTypeParameters(generic *types.GenericType, args []types.Type) bool {
	if len(args) != len(generic.TypeParameters) || generic.Body == nil {
		return false
	}
	for i, arg := range args {
		tp, ok := arg.(*types.TypeParameterType)
		if !ok || tp.Parameter != generic.TypeParameters[i] {
			return false
		}
	}
	return true
}

// substituteTypes recursively substitutes TypeParameterType with concrete types
func (c *Checker) substituteTypes(t types.Type, substitution map[string]types.Type) types.Type {
	return c.substituteTypesWithVisited(t, substitution, make(map[types.Type]types.Type))
}

func (c *Checker) substituteTypesWithVisited(t types.Type, substitution map[string]types.Type, visited map[types.Type]types.Type) types.Type {
	if t == nil {
		return nil
	}

	switch typ := t.(type) {
	case *types.TypeParameterType:
		// Replace type parameter with concrete type
		if replacement, exists := substitution[typ.Parameter.Name]; exists {
			if checkerDebug {
				debugPrintf("// [Checker] Substituting %s -> %s\n", typ.Parameter.Name, replacement.String())
			}
			return replacement
		}
		// Type parameter not found in substitution - this shouldn't happen if validation is correct
		debugPrintf("// [Checker] WARNING: Type parameter '%s' not found in substitution map\n", typ.Parameter.Name)
		return typ

	case *types.ForwardReferenceType:
		// Forward references should not appear in instantiated types, but handle gracefully
		debugPrintf("// [Checker] WARNING: ForwardReferenceType '%s' found during type substitution\n", typ.ClassName)
		// Return as-is for now - this indicates the forward reference wasn't properly resolved
		return typ

	case *types.ParameterizedForwardReferenceType:
		// Substitute type arguments in the parameterized forward reference
		newTypeArgs := make([]types.Type, len(typ.TypeArguments))
		for i, argType := range typ.TypeArguments {
			newTypeArgs[i] = c.substituteTypesWithVisited(argType, substitution, visited)
		}

		// Return a new parameterized forward reference with substituted type arguments
		return &types.ParameterizedForwardReferenceType{
			ClassName:     typ.ClassName,
			TypeArguments: newTypeArgs,
		}

	case *types.InstantiatedType:
		// `Promise<T>` inside a generic alias body: substitute the arguments
		newArgs := make([]types.Type, len(typ.TypeArguments))
		changed := false
		for i, arg := range typ.TypeArguments {
			newArgs[i] = c.substituteTypesWithVisited(arg, substitution, visited)
			if newArgs[i] != arg {
				changed = true
			}
		}
		if !changed {
			return typ
		}
		return types.NewInstantiatedType(typ.Generic, newArgs)

	case *types.GenericTypeAliasForwardReference:
		changed := false
		args := make([]types.Type, len(typ.TypeArguments))
		for k, arg := range typ.TypeArguments {
			if arg == nil {
				return typ
			}
			args[k] = c.substituteTypesWithVisited(arg, substitution, visited)
			changed = changed || args[k] != arg
		}
		if !changed {
			return typ
		}
		return &types.GenericTypeAliasForwardReference{AliasName: typ.AliasName, TypeArguments: args, Scope: typ.Scope}
	case *types.ReadonlyType:
		inner := c.substituteTypesWithVisited(typ.InnerType, substitution, visited)
		if inner == typ.InnerType {
			return typ
		}
		return types.NewReadonlyType(inner)
	case *types.ArrayType:
		// Recursively substitute element type
		newElementType := c.substituteTypesWithVisited(typ.ElementType, substitution, visited)
		return &types.ArrayType{ElementType: newElementType}

	case *types.ObjectType:
		if replacement, exists := visited[typ]; exists {
			return replacement
		}
		if c.instantiationUnchanged(typ, substitution, visited) {
			visited[typ] = typ
			return typ
		}
		if mentions, seen := c.objectMayMention(typ, substitution); !mentions {
			// Nothing to substitute: keep the object rather than deep-copy
			// it (instantiating zod's aliases copied whole class types).
			// The walk that found no mention explored everything reachable
			// from every object it entered, so substitution keeps those too:
			// asking again per object rewalked (and materialized) the same
			// graph, quadratic on playwright's types with @types/node.
			for kept := range seen {
				if _, done := visited[kept]; !done {
					visited[kept] = kept
				}
			}
			return typ
		}
		if typ.Pending {
			// A class its declaration fills in later, in place: keep the
			// reference, a copy would stay empty.
			return typ
		}

		// The copy's members are substituted when first read: most members
		// of most instantiations (a method's parameter types in a .d.ts) are
		// never read.
		result := &types.ObjectType{
			PropertyDocs:       typ.PropertyDocs,
			Doc:                typ.Doc,
			IsReflectIntrinsic: typ.IsReflectIntrinsic,
			IsInterface:        typ.IsInterface,
		}
		visited[typ] = result
		typ.CopyDisplay(result, func(a types.Type) types.Type { return c.substituteTypesWithVisited(a, substitution, visited) })
		if typ.ClassMeta != nil {
			// A full copy: SourceClassName and the inheritance fields drive
			// protected/private access checks on an instantiated generic class (#613).
			meta := *typ.ClassMeta
			result.ClassMeta = &meta
		}
		if c.closedInstantiation(typ, substitution) {
			result.Instantiates = typ.Instantiates
		}
		// The caller may reuse its map for another substitution.
		subst, env := maps.Clone(substitution), c.env
		result.SetLazyMembers(func(ot *types.ObjectType) {
			// Substitute as where the instantiation was written (`typeof x`
			// resolves in that scope); what it reports was reported there.
			saved := c.env
			c.env = env
			c.speculate(func() { c.substituteMembers(typ, ot, subst, visited) })
			c.env = saved
		})
		return result

	case *types.UnionType:
		// Recursively substitute union member types
		newTypes := make([]types.Type, len(typ.Types))
		for i, memberType := range typ.Types {
			newTypes[i] = c.substituteTypesWithVisited(memberType, substitution, visited)
		}
		return types.NewUnionType(newTypes...)

	case *types.IntersectionType:
		// Recursively substitute intersection member types
		newTypes := make([]types.Type, len(typ.Types))
		for i, memberType := range typ.Types {
			newTypes[i] = c.substituteTypesWithVisited(memberType, substitution, visited)
		}
		return types.NewIntersectionType(newTypes...)

	case *types.TupleType:
		// Recursively substitute tuple element types
		newElementTypes := make([]types.Type, len(typ.ElementTypes))
		for i, elementType := range typ.ElementTypes {
			newElementTypes[i] = c.substituteTypesWithVisited(elementType, substitution, visited)
		}
		var newRest types.Type
		if typ.RestElementType != nil {
			newRest = c.substituteTypesWithVisited(typ.RestElementType, substitution, visited)
		}
		return &types.TupleType{ElementTypes: newElementTypes, OptionalElements: typ.OptionalElements, RestElementType: newRest}

	case *types.MappedType:
		// Recursively substitute types in mapped type
		var newConstraintType types.Type
		if keyof, ok := typ.ConstraintType.(*types.KeyofType); ok {
			// Keep `keyof T` unevaluated once T is an object: the mapped type
			// is homomorphic and expansion copies T's `?` / readonly
			// modifiers, which the evaluated key union would lose (#638).
			operand := c.substituteTypesWithVisited(keyof.OperandType, substitution, visited)
			if _, isObj := c.structuralObject(operand); isObj {
				newConstraintType = &types.KeyofType{OperandType: operand}
			} else {
				newConstraintType = c.computeKeyofType(operand)
			}
		} else {
			newConstraintType = c.substituteTypesWithVisited(typ.ConstraintType, substitution, visited)
		}
		// The value means the mapped type's own key, not an outer one.
		newValueType := typ.ValueType
		if unshadowed, shadows := unshadowedNames(substitution, []*types.TypeParameter{{Name: typ.TypeParameter}}); !shadows {
			newValueType = c.substituteTypesWithVisited(typ.ValueType, substitution, visited)
		} else if len(unshadowed) > 0 {
			newValueType = c.substituteTypesWithVisited(typ.ValueType, unshadowed, make(map[types.Type]types.Type))
		}

		return &types.MappedType{
			TypeParameter:    typ.TypeParameter, // Keep parameter name as-is
			ConstraintType:   newConstraintType,
			ValueType:        newValueType,
			OptionalModifier: typ.OptionalModifier,
			ReadonlyModifier: typ.ReadonlyModifier,
		}

	case *types.KeyofType:
		// Substitute the operand type
		newOperandType := c.substituteTypesWithVisited(typ.OperandType, substitution, visited)
		// Compute the keyof type after substitution
		return c.computeKeyofType(newOperandType)

	case *types.IndexedAccessType:
		// Substitute both object and index types
		newObjectType := c.substituteTypesWithVisited(typ.ObjectType, substitution, visited)
		newIndexType := c.substituteTypesWithVisited(typ.IndexType, substitution, visited)
		// Once both sides are concrete, T[K] is just a property type, also for
		// a mapped object or a union key (`{ [k in keyof T]: ... }[keyof T]`).
		_, objIsParam := newObjectType.(*types.TypeParameterType)
		_, idxIsParam := newIndexType.(*types.TypeParameterType)
		if !objIsParam && !idxIsParam {
			if resolved := c.computeIndexedAccessType(newObjectType, newIndexType); resolved != nil {
				return resolved
			}
		}
		return &types.IndexedAccessType{
			ObjectType: newObjectType,
			IndexType:  newIndexType,
		}

	case *types.ConditionalType:
		// Substitute check type and false type
		newCheckType := c.substituteTypesWithVisited(typ.CheckType, substitution, visited)
		newFalseType := c.substituteTypesWithVisited(typ.FalseType, substitution, visited)

		// For extends type, don't substitute infer types - they should remain as inference placeholders
		newExtendsType := c.substituteTypesPreservingInferVisited(typ.ExtendsType, substitution, visited)

		// For true type, don't substitute infer types - they will be replaced by inference results
		newTrueType := c.substituteTypesPreservingInferVisited(typ.TrueType, substitution, visited)

		// Try to compute the result with substituted types
		debugPrintf("// [SubstituteTypes] About to compute conditional type...\n")
		resolvedType := c.computeConditionalType(newCheckType, newExtendsType, newTrueType, newFalseType)
		if resolvedType != nil {
			if checkerDebug {
				debugPrintf("// [SubstituteTypes] Conditional type resolved to: %s\n", resolvedType.String())
			}
			return resolvedType
		}
		debugPrintf("// [SubstituteTypes] Conditional type computation returned nil, keeping conditional type\n")

		// Return the conditional type with substituted parts
		return &types.ConditionalType{
			CheckType:   newCheckType,
			ExtendsType: newExtendsType,
			TrueType:    newTrueType,
			FalseType:   newFalseType,
		}

	case *types.TemplateLiteralType:
		// Substitute types in template literal type parts
		newParts := make([]types.TemplateLiteralPart, len(typ.Parts))
		for i, part := range typ.Parts {
			if part.IsLiteral {
				// String literals don't need substitution
				newParts[i] = part
			} else {
				// Substitute the type in type interpolation parts
				newType := c.substituteTypesWithVisited(part.Type, substitution, visited)
				newParts[i] = types.TemplateLiteralPart{
					IsLiteral: false,
					Literal:   "",
					Type:      newType,
				}
			}
		}

		// Try to compute the template literal type with substituted parts
		substitutedTlt := &types.TemplateLiteralType{Parts: newParts}
		computedType := c.computeTemplateLiteralType(substitutedTlt)
		if computedType != nil {
			return computedType
		}

		return substitutedTlt

	default:
		// For primitive types and other types that don't contain type parameters,
		// return as-is
		return typ
	}
}

// resolveKeyofTypeExpression resolves a keyof type expression to a union of string literals
func (c *Checker) resolveKeyofTypeExpression(node *parser.KeyofTypeExpression) types.Type {
	if node.Type == nil {
		c.addError(node, "keyof expression missing operand type")
		return nil
	}

	operandType := c.resolveTypeAnnotation(node.Type)
	if operandType == nil {
		// Error already reported by resolveTypeAnnotation
		return nil
	}

	if _, ok := operandType.(*types.TypeofType); ok {
		// `type K = keyof typeof C` hoisted above `const C = {...}`: keyed
		// once C has a type (resolveTypeofInType), not never now.
		return &types.KeyofType{OperandType: operandType}
	}
	// Compute the actual keyof type by extracting keys from the operand type
	return c.computeKeyofType(operandType)
}

// resolveTypeofImport resolves `typeof import("x").a.b`: the module namespace
// object, then its members.
func (c *Checker) resolveTypeofImport(node *parser.TypeofTypeExpression) types.Type {
	ns := c.importTypeNamespace(node.Import)
	if ns == nil {
		return types.Any
	}
	var t types.Type = ns.ValueShape
	for _, name := range node.Path {
		member := c.getPropertyTypeFromType(t, name, false)
		if member == nil {
			c.addErrorWithCode(node, errors.TS2339, fmt.Sprintf("Property '%s' does not exist on type '%s'.", name, t.String()))
			return types.Any
		}
		t = member
	}
	return t
}

// resolveTypeofTypeExpression resolves a typeof type expression to the type of a variable.
// Handles both simple names ('typeof x') and dotted paths ('typeof M2.Point').
func (c *Checker) resolveTypeofTypeExpression(node *parser.TypeofTypeExpression) types.Type {
	if node.Import != nil {
		return c.resolveTypeofImport(node)
	}
	path := node.Path
	if len(path) == 0 {
		path = []string{node.Identifier}
	}
	debugPrintf("// [Checker resolveTypeofType] Resolving typeof %v\n", path)

	if len(path) == 0 || path[0] == "" {
		c.addError(node, "typeof expression missing identifier")
		return nil
	}

	// Resolve the first segment from the environment.
	c.inferPendingInit(path[0])
	varType, _, found := c.env.Resolve(path[0])
	if found {
		if c.isBuiltinGlobalThis(path[0], varType) {
			varType = c.globalThisType()
		}
		varType = c.checkNamespaceUsedAsValue(&parser.Identifier{Token: node.Token, Value: path[0]}, varType)
	}
	if !found {
		if len(path) == 1 {
			c.unresolvedTypeofNodes = append(c.unresolvedTypeofNodes, node)
			c.unresolvedTypeofTypeOnly = append(c.unresolvedTypeofTypeOnly, c.isTypeOnlyName(node.Identifier))
			return &types.TypeofType{Identifier: node.Identifier}
		}
		// Not declared yet (a hoisted annotation): resolve it later.
		c.unresolvedTypeofNodes = append(c.unresolvedTypeofNodes, node)
		c.unresolvedTypeofTypeOnly = append(c.unresolvedTypeofTypeOnly, false)
		return &types.TypeofType{Identifier: path[0], Path: path}
	}

	varType, ok := c.walkTypeofPath(node, path, varType)
	if !ok {
		return nil
	}

	debugPrintf("// [Checker resolveTypeofType] Found type for '%v': %T\n", path, varType)
	return varType
}

// resolveTypeofTypeIfNeeded resolves a TypeofType forward reference if needed
func (c *Checker) resolveTypeofTypeIfNeeded(t types.Type) types.Type {
	if typeofType, ok := t.(*types.TypeofType); ok {
		debugPrintf("// [Checker resolveTypeofTypeIfNeeded] Resolving forward reference for typeof %s\n", typeofType.Identifier)

		// Look up the identifier in the environment
		c.inferPendingInit(typeofType.Identifier)
		varType, _, found := c.env.Resolve(typeofType.Identifier)
		if found {
			if c.isBuiltinGlobalThis(typeofType.Identifier, varType) {
				varType = c.globalThisType()
			}
			debugPrintf("// [Checker resolveTypeofTypeIfNeeded] Successfully resolved typeof %s to %T\n", typeofType.Identifier, varType)
			if len(typeofType.Path) > 1 {
				walked, ok := c.speculativeTypeofPath(typeofType.Path, varType)
				if !ok {
					return typeofType
				}
				return walked
			}
			return varType
		} else {
			debugPrintf("// [Checker resolveTypeofTypeIfNeeded] Still cannot resolve typeof %s, keeping forward reference\n", typeofType.Identifier)
			// Keep the forward reference for now
			return typeofType
		}
	}
	// Not a TypeofType, return as-is
	return c.resolveDeferredTypeofIndex(t, false)
}

// resolveDeferredTypeofIndex resolves `(typeof X)[K]`, alone or in a union,
// that was written before X had a type (a hoisted signature or type alias)
// and so was left as an IndexedAccessType (#638).
//
// With anyIsFinal, a `typeof X` that still resolves to any (X read from a
// hoisted function body before its initializer was checked) indexes to any;
// otherwise it stays deferred so a later use can see X's real type.
func (c *Checker) resolveDeferredTypeofIndex(t types.Type, anyIsFinal bool) types.Type {
	switch tt := t.(type) {
	case *types.IndexedAccessType:
		if _, ok := tt.ObjectType.(*types.TypeofType); ok {
			if resolved := c.computeIndexedAccessType(tt.ObjectType, tt.IndexType); resolved != nil {
				return resolved
			}
			if anyIsFinal && c.resolveTypeofTypeIfNeeded(tt.ObjectType) == types.Any {
				return types.Any
			}
		}
	case *types.UnionType:
		changed := false
		members := make([]types.Type, len(tt.Types))
		for i, m := range tt.Types {
			members[i] = c.resolveDeferredTypeofIndex(m, anyIsFinal)
			changed = changed || members[i] != m
		}
		if changed {
			return types.NewUnionType(members...)
		}
	}
	return t
}

// resolveInferTypeExpression resolves an infer type expression
func (c *Checker) resolveInferTypeExpression(node *parser.InferTypeExpression) types.Type {
	if node.TypeParameter == "" {
		c.addError(node, "infer expression missing type parameter")
		return nil
	}

	// Create an InferType that represents the inferred type placeholder
	// The actual inference happens during conditional type resolution
	return &types.InferType{
		TypeParameter: node.TypeParameter,
	}
}

// computeKeyofType computes the keyof type for a given type
func (c *Checker) computeKeyofType(operandType types.Type) types.Type {
	switch typ := operandType.(type) {
	case *types.ObjectType:
		// Property names as string literals, in declaration order, then
		// inherited ones; a string index signature adds string | number.
		var keyTypes []types.Type
		seen := map[string]bool{}
		add := func(name string) {
			if !seen[name] {
				seen[name] = true
				keyTypes = append(keyTypes, &types.LiteralType{Value: vm.String(name)})
			}
		}
		for _, name := range typ.PropertyNames() {
			add(name)
		}
		if len(typ.Bases()) > 0 {
			inherited := typ.GetEffectiveProperties()
			names := make([]string, 0, len(inherited))
			for name := range inherited {
				names = append(names, name)
			}
			sort.Strings(names)
			for _, name := range names {
				add(name)
			}
		}
		for _, sig := range typ.IndexSigs() {
			if sig == nil {
				continue
			}
			switch sig.KeyType {
			case types.String:
				keyTypes = append(keyTypes, types.String, types.Number)
			case types.Number:
				keyTypes = append(keyTypes, types.Number)
			}
		}
		if len(keyTypes) == 0 {
			return types.Never
		}
		if len(keyTypes) == 1 {
			return keyTypes[0]
		}
		return types.NewUnionType(keyTypes...)

	case *types.MappedType:
		// keyof { [P in K]: X } is K; expand first so `keyof T` constraints
		// that are concrete by now resolve.
		if expanded := c.expandMappedType(typ); expanded != nil {
			if obj, ok := expanded.(*types.ObjectType); ok {
				return c.computeKeyofType(obj)
			}
		}
		if typ.ConstraintType != nil {
			return typ.ConstraintType
		}
		return &types.KeyofType{OperandType: operandType}

	case *types.IntersectionType:
		// keyof (A & B) = keyof A | keyof B
		var keys []types.Type
		for _, member := range typ.Types {
			k := c.computeKeyofType(member)
			if _, deferred := k.(*types.KeyofType); deferred {
				return &types.KeyofType{OperandType: operandType}
			}
			if k != types.Never {
				keys = append(keys, k)
			}
		}
		if len(keys) == 0 {
			return types.Never
		}
		return types.NewUnionType(keys...)

	case *types.UnionType:
		// keyof (A | B) = the keys A and B share
		var common []types.Type
		for i, member := range typ.Types {
			k := c.computeKeyofType(member)
			if _, deferred := k.(*types.KeyofType); deferred {
				return &types.KeyofType{OperandType: operandType}
			}
			if i == 0 {
				common = keyMembers(k)
				continue
			}
			mine := keyMembers(k)
			var kept []types.Type
			for _, ck := range common {
				for _, mk := range mine {
					if ck.Equals(mk) || (mk == types.String && isStringLiteral(ck)) {
						kept = append(kept, ck)
						break
					}
				}
			}
			common = kept
		}
		if len(common) == 0 {
			return types.Never
		}
		return types.NewUnionType(common...)

	default:
		// Handle special cases
		if operandType == types.Any {
			// keyof any should be string | number | symbol (simplified to string for now)
			return types.String
		}
		switch operandType.(type) {
		case *types.TypeParameterType, *types.ForwardReferenceType,
			*types.TypeAliasForwardReference, *types.GenericTypeAliasForwardReference,
			*types.ParameterizedForwardReferenceType, *types.InstantiatedType,
			*types.ConditionalType, *types.IndexedAccessType:
			// Generic or unresolved operands may gain concrete properties during
			// substitution. Preserve keyof so mapped types like
			// { [P in keyof T]: T[P] } can expand after T is instantiated.
			return &types.KeyofType{OperandType: operandType}
		}
		// For non-object types, keyof typically resolves to never
		// TODO: Handle other types like arrays (which should include numeric indices)
		return types.Never
	}
}

// keyMembers lists a keyof result's members.
func keyMembers(k types.Type) []types.Type {
	if u, ok := k.(*types.UnionType); ok {
		return u.Types
	}
	if k == types.Never {
		return nil
	}
	return []types.Type{k}
}

func isStringLiteral(t types.Type) bool {
	lit, ok := t.(*types.LiteralType)
	return ok && lit.Value.Type() == vm.TypeString
}

// resolveTypePredicateExpression resolves a type predicate expression to a TypePredicateType
func (c *Checker) resolveTypePredicateExpression(node *parser.TypePredicateExpression) types.Type {
	if node.Parameter == nil {
		c.addError(node, "type predicate missing parameter name")
		return nil
	}

	if node.Type == nil {
		c.addError(node, "type predicate missing type")
		return nil
	}

	predicateType := c.resolveTypeAnnotation(node.Type)
	if predicateType == nil {
		// Error already reported by resolveTypeAnnotation
		return nil
	}

	return &types.TypePredicateType{
		ParameterName: node.Parameter.Value,
		Type:          predicateType,
		Asserts:       node.Asserts,
	}
}

// resolveMappedTypeExpression resolves a mapped type expression to a MappedType
func (c *Checker) resolveMappedTypeExpression(node *parser.MappedTypeExpression) types.Type {
	if node.TypeParameter == nil {
		c.addError(node, "mapped type missing type parameter")
		return nil
	}

	if node.ConstraintType == nil {
		c.addError(node, "mapped type missing constraint type")
		return nil
	}

	if node.ValueType == nil {
		c.addError(node, "mapped type missing value type")
		return nil
	}

	// Resolve constraint type (the type being iterated over)
	constraintType := c.resolveTypeAnnotation(node.ConstraintType)
	if constraintType == nil {
		// Error already reported by resolveTypeAnnotation
		return nil
	}

	// Create a temporary environment with the type parameter in scope
	// This allows expressions like T[P] to resolve P correctly
	originalEnv := c.env
	tempEnv := NewEnclosedEnvironment(c.env)

	// Add the type parameter to the temporary environment
	// For now, we'll represent it as a type parameter type
	typeParam := &types.TypeParameter{
		Name:       node.TypeParameter.Value,
		Constraint: constraintType, // The constraint is what P extends/iterates over
		Default:    nil,            // Mapped types don't have defaults
	}
	tempEnv.DefineTypeParameter(node.TypeParameter.Value, typeParam)

	// Switch to the temporary environment for resolving the value type
	c.env = tempEnv
	valueType := c.resolveTypeAnnotation(node.ValueType)
	c.env = originalEnv // Restore original environment

	if valueType == nil {
		// Error already reported by resolveTypeAnnotation
		return nil
	}

	// `[k in keyof X]` over a concrete X is homomorphic: X's members keep
	// their `?` and `readonly` (zod's flatten over addQuestionMarks), so the
	// constraint stays `keyof X` for the expansion to see X.
	mappedConstraint := constraintType
	if keyofNode, ok := node.ConstraintType.(*parser.KeyofTypeExpression); ok {
		if operand := c.resolveTypeAnnotation(keyofNode.Type); operand != nil {
			if _, ok := c.structuralObject(operand); ok {
				mappedConstraint = &types.KeyofType{OperandType: operand}
			}
		}
	}

	return &types.MappedType{
		TypeParameter:    node.TypeParameter.Value,
		ConstraintType:   mappedConstraint,
		ValueType:        valueType,
		ReadonlyModifier: node.ReadonlyModifier,
		OptionalModifier: node.OptionalModifier,
	}
}

// resolveIndexedAccessTypeExpression resolves indexed access types like T[K]
func (c *Checker) resolveIndexedAccessTypeExpression(node *parser.IndexedAccessTypeExpression) types.Type {
	if node.ObjectType == nil {
		c.addError(node, "indexed access type missing object type")
		return nil
	}

	if node.IndexType == nil {
		c.addError(node, "indexed access type missing index type")
		return nil
	}

	// Resolve the object type being indexed into
	objectType := c.resolveTypeAnnotation(node.ObjectType)
	if objectType == nil {
		// Error already reported by resolveTypeAnnotation
		return nil
	}

	// Resolve the index type used for accessing
	indexType := c.resolveTypeAnnotation(node.IndexType)
	if indexType == nil {
		// Error already reported by resolveTypeAnnotation
		return nil
	}

	// Try to compute the result if possible
	resolvedType := c.computeIndexedAccessType(objectType, indexType)
	if resolvedType != nil {
		return resolvedType
	}

	// If we can't resolve it now, return an IndexedAccessType for later resolution
	return &types.IndexedAccessType{
		ObjectType: objectType,
		IndexType:  indexType,
	}
}

// resolveConditionalTypeExpression resolves a conditional type expression to a ConditionalType
func (c *Checker) resolveConditionalTypeExpression(node *parser.ConditionalTypeExpression) types.Type {
	if node.CheckType == nil {
		c.addError(node, "conditional type missing check type")
		return nil
	}

	if node.ExtendsType == nil {
		c.addError(node, "conditional type missing extends type")
		return nil
	}

	if node.TrueType == nil {
		c.addError(node, "conditional type missing true type")
		return nil
	}

	if node.FalseType == nil {
		c.addError(node, "conditional type missing false type")
		return nil
	}

	// Resolve all component types
	checkType := c.resolveTypeAnnotation(node.CheckType)
	if checkType == nil {
		// Error already reported by resolveTypeAnnotation
		return nil
	}

	extendsType := c.resolveTypeAnnotation(node.ExtendsType)
	if extendsType == nil {
		// Error already reported by resolveTypeAnnotation
		return nil
	}

	var trueType types.Type
	c.withInferScope(node.ExtendsType, func() {
		trueType = c.resolveTypeAnnotation(node.TrueType)
	})
	if trueType == nil {
		// Error already reported by resolveTypeAnnotation
		return nil
	}

	falseType := c.resolveTypeAnnotation(node.FalseType)
	if falseType == nil {
		// Error already reported by resolveTypeAnnotation
		return nil
	}

	// Always return a ConditionalType for later resolution/substitution
	// The computation will happen during type substitution when generics are instantiated
	return &types.ConditionalType{
		CheckType:   checkType,
		ExtendsType: extendsType,
		TrueType:    trueType,
		FalseType:   falseType,
	}
}

// computeConditionalType computes the result of a conditional type like T extends U ? X : Y
// Enhanced to handle infer types for type inference
func (c *Checker) computeConditionalType(checkType, extendsType, trueType, falseType types.Type) types.Type {
	if checkType == nil || extendsType == nil || trueType == nil || falseType == nil {
		return nil // a part failed to resolve: nothing to decide
	}
	debugPrintf("// [ConditionalType] === STARTING CONDITIONAL TYPE COMPUTATION ===\n")
	if checkerDebug {
		debugPrintf("// [ConditionalType] CheckType: %s (Go type: %T)\n", checkType.String(), checkType)
	}
	if checkerDebug {
		debugPrintf("// [ConditionalType] ExtendsType: %s (Go type: %T)\n", extendsType.String(), extendsType)
	}
	if checkerDebug {
		debugPrintf("// [ConditionalType] TrueType: %s (Go type: %T)\n", trueType.String(), trueType)
	}
	if checkerDebug {
		debugPrintf("// [ConditionalType] FalseType: %s (Go type: %T)\n", falseType.String(), falseType)
	}

	// Distributive conditional types: when checkType is a union, distribute over its members
	// This implements TypeScript's distributive conditional types behavior
	// e.g., ("a" | "b" | "c") extends "b" ? never : T  =>  ("a" extends "b" ? never : "a") | ("b" extends "b" ? never : "b") | ("c" extends "b" ? never : "c")
	// When the falseType equals the original checkType (as in Exclude<T, U> = T extends U ? never : T),
	// each distributed member replaces T in the false branch.
	if unionCheck, ok := checkType.(*types.UnionType); ok {
		var resultTypes []types.Type
		for _, member := range unionCheck.Types {
			// Substitute: if falseType is the same union as checkType, use the member
			effectiveFalse := falseType
			if falseType != nil && falseType.Equals(checkType) {
				effectiveFalse = member
			}
			effectiveTrue := trueType
			if trueType != nil && trueType != types.Never && trueType.Equals(checkType) {
				effectiveTrue = member
			}
			result := c.computeConditionalType(member, extendsType, effectiveTrue, effectiveFalse)
			if result == nil {
				return nil // a member cannot be decided yet: neither can the union
			}
			if result != types.Never {
				resultTypes = append(resultTypes, result)
			}
		}
		if len(resultTypes) == 0 {
			return types.Never
		}
		return types.NewUnionType(resultTypes...)
	}

	// T[K] with T or K still a type parameter (a mapped type's value before
	// its key is substituted) isn't known yet: defer, rather than let the
	// unresolved indexed access count as assignable and pick the true branch.
	// The same holds on the extends side: zod's optionalKeys is
	// `{ [k in keyof T]: undefined extends T[k] ? k : never }`. A bare type
	// parameter on either side defers too: zod's CatchallOutput<T> is
	// `ZodType extends T ? unknown : ...`, decided only once T is known.
	for _, side := range []types.Type{checkType, extendsType} {
		if _, isParam := side.(*types.TypeParameterType); isParam {
			return nil
		}
		if ia, ok := side.(*types.IndexedAccessType); ok {
			_, objIsParam := ia.ObjectType.(*types.TypeParameterType)
			_, idxIsParam := ia.IndexType.(*types.TypeParameterType)
			if objIsParam || idxIsParam {
				return nil
			}
		}
	}

	// A reference to a generic declared later (ZodTypeAny = ZodType<any, any,
	// any>, resolved before class ZodType) is instantiated now if it can be,
	// and otherwise the conditional waits: deciding against a placeholder
	// picks the wrong branch.
	for _, side := range []*types.Type{&checkType, &extendsType} {
		ref, ok := (*side).(*types.GenericTypeAliasForwardReference)
		if !ok {
			continue
		}
		if inst := c.instantiateAliasReference(ref); inst != nil {
			*side = inst
		} else if c.declaredTypeNames[ref.AliasName] {
			return nil
		}
	}

	// A side that still mentions a class or generic not declared yet (a
	// hoisted alias naming ZodOptional<ZodNumber>) cannot be judged; settleType
	// evaluates it where the alias is used.
	if c.mentionsUndeclared(checkType, 0) || c.mentionsUndeclared(extendsType, 0) {
		return nil
	}

	// Check if we have unresolved TypeofType that we should defer
	if c.containsUnresolvedTypeofType(checkType) || c.containsUnresolvedTypeofType(extendsType) {
		debugPrintf("// [ConditionalType] Contains unresolved TypeofType, deferring evaluation\n")
		// Return the conditional type as-is to be resolved later
		return &types.ConditionalType{
			CheckType:   checkType,
			ExtendsType: extendsType,
			TrueType:    trueType,
			FalseType:   falseType,
		}
	}

	// Resolve typeof forward references before processing
	checkType = c.resolveTypeofTypeIfNeeded(checkType)
	extendsType = c.resolveTypeofTypeIfNeeded(extendsType)

	if checkerDebug {
		debugPrintf("// [ConditionalType] After resolving typeof: CheckType: %s (Go type: %T)\n", checkType.String(), checkType)
	}
	if checkerDebug {
		debugPrintf("// [ConditionalType] After resolving typeof: ExtendsType: %s (Go type: %T)\n", extendsType.String(), extendsType)
	}

	// Check if the extends type contains infer types that we need to match
	inferences := make(map[string]types.Type)
	if c.inferFromConditional(checkType, extendsType, inferences) {
		if checkerDebug {
			debugPrintf("// [ConditionalType] Inference successful with %d captured types\n", len(inferences))
		}

		// `any extends U ? X : Y` is both branches: NonNullable<any> is any.
		if checkType == types.Any && len(inferences) == 0 {
			return types.NewUnionType(c.inferAsAny(trueType), falseType)
		}

		// Apply the inferences to the true type
		substitutedTrueType := c.substituteInferredTypes(trueType, inferences)
		if checkerDebug {
			debugPrintf("// [ConditionalType] YES with inference: %s extends %s -> %s\n",
				checkType.String(), extendsType.String(), substitutedTrueType.String())
		}
		return substitutedTrueType
	}

	// `any` matches any pattern: both branches, with `infer X` taken as any.
	if checkType == types.Any {
		return types.NewUnionType(c.inferAsAny(trueType), falseType)
	}

	// Fallback to basic assignability check without inference
	if types.IsAssignable(checkType, extendsType) {
		if checkerDebug {
			debugPrintf("// [ConditionalType] YES: %s extends %s -> %s\n", checkType.String(), extendsType.String(), trueType.String())
		}
		return trueType
	} else {
		if checkerDebug {
			debugPrintf("// [ConditionalType] NO: %s does not extend %s -> %s\n", checkType.String(), extendsType.String(), falseType.String())
		}
		return falseType
	}
}

// computeIndexedAccessType computes the result of an indexed access type like T[K]
func (c *Checker) computeIndexedAccessType(objectType, indexType types.Type) types.Type {
	// `(typeof X)[K]` written before X had a type: index X's type now.
	if _, ok := objectType.(*types.TypeofType); ok {
		objectType = c.resolveTypeofTypeIfNeeded(objectType)
		if _, still := objectType.(*types.TypeofType); still {
			return nil
		}
	}
	// (A & B)[K] is A[K] & B[K] over the members that have K; (A | B)[K] is
	// A[K] | B[K] and needs K in every member.
	switch ot := objectType.(type) {
	case *types.IntersectionType:
		var parts []types.Type
		for _, member := range ot.Types {
			if t := c.computeIndexedAccessType(member, indexType); t != nil {
				parts = append(parts, t)
			}
		}
		if len(parts) == 0 {
			return nil
		}
		return types.NewIntersectionType(parts...)
	case *types.UnionType:
		parts := make([]types.Type, 0, len(ot.Types))
		for _, member := range ot.Types {
			t := c.computeIndexedAccessType(member, indexType)
			if t == nil {
				return nil
			}
			parts = append(parts, t)
		}
		return types.NewUnionType(parts...)
	}
	// `Required<any>[number]`: a mapped type is indexed through its expansion.
	if mapped, ok := objectType.(*types.MappedType); ok {
		if expanded := c.expandMappedType(mapped); expanded != nil {
			objectType = expanded
		}
	}
	// A primitive key reads the matching index signature.
	if obj, ok := objectType.(*types.ObjectType); ok && (indexType == types.String || indexType == types.Number) {
		for _, sig := range obj.IndexSigs() {
			if sig.KeyType == indexType || (indexType == types.Number && sig.KeyType == types.String) {
				return sig.ValueType
			}
		}
	}
	// Arrays and tuples, readonly or not, indexed by a number: `T[][0]`,
	// `(typeof TUPLE)[number]` (#638).
	elementsOf := c.resolveTypeofTypeIfNeeded(objectType)
	if ro, ok := elementsOf.(*types.ReadonlyType); ok {
		elementsOf = ro.InnerType
	}
	if isNumericIndexType(indexType) {
		switch ot := elementsOf.(type) {
		case *types.ArrayType:
			return ot.ElementType
		case *types.TupleType:
			if lit, ok := indexType.(*types.LiteralType); ok {
				i := int(lit.Value.ToFloat())
				if float64(i) == lit.Value.ToFloat() && i >= 0 && i < len(ot.ElementTypes) {
					elem := ot.ElementTypes[i]
					if i < len(ot.OptionalElements) && ot.OptionalElements[i] {
						elem = types.NewUnionType(elem, types.Undefined)
					}
					return elem
				}
				if ot.RestElementType != nil && i >= len(ot.ElementTypes) {
					return ot.RestElementType
				}
				return nil
			}
			return getTupleElementUnion(ot)
		}
	}

	// Handle object types with specific string literal keys
	if objType, ok := objectType.(*types.ObjectType); ok {
		// Include members inherited from base types (classes, interfaces)
		props := objType.GetEffectiveProperties()
		// Case: Object["propertyName"] where "propertyName" is a string literal
		if literalType, ok := indexType.(*types.LiteralType); ok {
			if literalType.Value.Type() == vm.TypeString {
				strVal := literalType.Value.AsString()
				// Look up the property directly
				if propType, exists := props[strVal]; exists {
					return propType
				}
				// Property doesn't exist - this could be an error or return never/undefined
				// For now, return nil to indicate it couldn't be resolved
				return nil
			}
		}

		// Case: Object[keyof Object] - return union of all property types
		if keyofType, ok := indexType.(*types.KeyofType); ok {
			if keyofType.OperandType.Equals(objectType) {
				// Collect all property types
				var propTypes []types.Type
				for _, propName := range types.SortedPropertyNames(props) {
					propTypes = append(propTypes, props[propName])
				}
				if len(propTypes) == 0 {
					return types.Never // No properties means never
				}
				if len(propTypes) == 1 {
					return propTypes[0] // Single property type
				}
				return types.NewUnionType(propTypes...) // Union of all property types
			}
		}

		// Case: Object[union of string literals] - return union of corresponding property types
		if unionType, ok := indexType.(*types.UnionType); ok {
			var resultTypes []types.Type
			for _, memberType := range unionType.Types {
				if literalType, ok := memberType.(*types.LiteralType); ok {
					if literalType.Value.Type() == vm.TypeString {
						strVal := literalType.Value.AsString()
						if propType, exists := props[strVal]; exists {
							resultTypes = append(resultTypes, propType)
						}
					}
				}
			}
			if len(resultTypes) == 0 {
				return nil // Couldn't resolve any properties
			}
			if len(resultTypes) == 1 {
				return resultTypes[0]
			}
			return types.NewUnionType(resultTypes...)
		}
	}

	// TODO: Handle other cases like:
	// - Array[number] should return the element type
	// - Tuple[number] should return union of tuple element types
	// - Generic type parameters T[K] with constraints

	// For now, return nil to indicate it couldn't be resolved immediately
	return nil
}

// expandMappedType expands a mapped type to a concrete ObjectType
// Example: { [P in keyof Person]?: Person[P] } → { name?: string; age?: number }
func (c *Checker) expandMappedType(mappedType *types.MappedType) types.Type {
	if mappedType == nil {
		return nil
	}

	// Get the constraint type (what we're iterating over)
	constraintType := mappedType.ConstraintType
	if constraintType == nil {
		return nil
	}

	// Handle keyof constraint: [P in keyof SomeType]
	var iterationKeys []types.Type
	var sourceObjectType *types.ObjectType // Track source object for inheriting optionality
	if keyofType, ok := constraintType.(*types.KeyofType); ok {
		// Get the keys from the keyof operand
		operandType := c.resolveStructural(keyofType.OperandType)
		if ref, ok := operandType.(*types.GenericTypeAliasForwardReference); ok {
			if inst := c.instantiateAliasReference(ref); inst != nil {
				operandType = c.resolveStructural(inst)
			}
		}
		if objType, ok := c.structuralObject(operandType); ok {
			sourceObjectType = objType
			// Extract all property names as literal types
			for _, propName := range objType.PropertyNames() {
				iterationKeys = append(iterationKeys, &types.LiteralType{
					Value: vm.String(propName),
				})
			}
		} else if operandType == types.Any {
			// For keyof any, we can't enumerate specific keys, so this mapped type
			// should act like any for property access - return Any
			return types.Any
		}
	} else if unionType, ok := constraintType.(*types.UnionType); ok {
		// Handle direct union constraint: [P in "name" | "age"]
		iterationKeys = unionType.Types
	} else if literalType, ok := constraintType.(*types.LiteralType); ok {
		// Handle single literal constraint: [P in "name"]
		iterationKeys = []types.Type{literalType}
	} else if constraintType == types.String {
		// Handle case where constraint is 'string' (e.g., Record<string, V>)
		// Create an ObjectType with a string index signature
		resultObj := types.NewObjectType()
		valueType := mappedType.ValueType
		// Substitute the type parameter with string in the value type
		if valueType != nil {
			valueType = c.substituteTypeParameterInType(valueType, mappedType.TypeParameter, types.String)
			if valueType == nil {
				valueType = types.Any
			}
			// Resolve IndexedAccessType: any[X] -> any
			if iat, ok := valueType.(*types.IndexedAccessType); ok {
				if iat.ObjectType == types.Any {
					valueType = types.Any
				}
			}
		} else {
			valueType = types.Any
		}
		resultObj.Materialize()
		resultObj.IndexSignatures = append(resultObj.IndexSigs(), &types.IndexSignature{
			KeyType:   types.String,
			ValueType: valueType,
		})
		return resultObj
	} else if constraintType == types.Number {
		// Handle case where constraint is 'number'
		resultObj := types.NewObjectType()
		valueType := mappedType.ValueType
		if valueType != nil {
			valueType = c.substituteTypeParameterInType(valueType, mappedType.TypeParameter, types.Number)
			if valueType == nil {
				valueType = types.Any
			}
		} else {
			valueType = types.Any
		}
		resultObj.Materialize()
		resultObj.IndexSignatures = append(resultObj.IndexSigs(), &types.IndexSignature{
			KeyType:   types.Number,
			ValueType: valueType,
		})
		return resultObj
	} else if condType, ok := constraintType.(*types.ConditionalType); ok {
		// Handle conditional type constraints like Exclude<keyof T, K>
		// Distribute the conditional type over its check type to compute the result
		var checkKeys []types.Type
		switch ct := condType.CheckType.(type) {
		case *types.KeyofType:
			if objType, ok := ct.OperandType.(*types.ObjectType); ok {
				sourceObjectType = objType
				for propName := range objType.Props() {
					checkKeys = append(checkKeys, &types.LiteralType{Value: vm.String(propName)})
				}
			}
		case *types.UnionType:
			checkKeys = ct.Types
		case *types.LiteralType:
			checkKeys = []types.Type{ct}
		}
		// Distribute: for each key, evaluate the conditional type
		for _, key := range checkKeys {
			extends := key.Equals(condType.ExtendsType)
			if !extends {
				// Check if key is a member of a union ExtendsType
				if unionExtends, ok := condType.ExtendsType.(*types.UnionType); ok {
					for _, member := range unionExtends.Types {
						if key.Equals(member) {
							extends = true
							break
						}
					}
				}
			}
			if extends {
				// Key extends the excluded type - use TrueType
				if condType.TrueType != types.Never {
					iterationKeys = append(iterationKeys, condType.TrueType)
				}
				// If TrueType is Never, this key is excluded (filtered out)
			} else {
				// Key does not extend the excluded type - use FalseType
				// For Exclude, FalseType is typically the key itself
				iterationKeys = append(iterationKeys, key)
			}
		}
	} else if constraintType == types.Never {
		// No keys: `{ [K in never]: X }` is `{}` (zod's addQuestionMarks
		// when every key is optional).
		return types.NewObjectType()
	} else {
		// Unsupported constraint type for now
		return nil
	}

	if len(iterationKeys) == 0 {
		if _, ok := constraintType.(*types.KeyofType); ok && sourceObjectType != nil {
			return types.NewObjectType() // keyof an object without members
		}
		return nil
	}

	// Create the expanded object type
	properties := make(map[string]types.Type)
	var propertyOrder []string
	optionalProperties := make(map[string]bool)

	readOnlyProperties := make(map[string]bool)
	// For each key in the iteration, compute the resulting property
	for _, keyType := range iterationKeys {
		literalType, ok := keyType.(*types.LiteralType)
		if !ok || literalType.Value.Type() != vm.TypeString {
			continue // Skip non-string keys for now
		}

		keyName := literalType.Value.AsString()

		// Compute the value type for this property
		// We need to substitute P with the current key in the value type
		valueType := c.substituteTypeParameterInType(
			mappedType.ValueType,
			mappedType.TypeParameter,
			keyType,
		)

		if valueType != nil {
			types.SetOrdered(properties, &propertyOrder, keyName, valueType)

			// Handle optional modifier
			if mappedType.OptionalModifier == "+" {
				// Explicitly make property optional
				optionalProperties[keyName] = true
			} else if mappedType.OptionalModifier == "-" {
				// Explicitly make property required (remove optional), and
				// with it the `undefined` the optionality contributed.
				optionalProperties[keyName] = false
				types.SetOrdered(properties, &propertyOrder, keyName, removeUndefinedMember(valueType))
			} else if mappedType.OptionalModifier == "" && sourceObjectType != nil {
				// No modifier: inherit optionality from source type
				if sourceObjectType.IsPropertyOptional(keyName) {
					optionalProperties[keyName] = true
				}
			}

			// Readonly modifier, likewise inherited by a homomorphic mapping.
			switch mappedType.ReadonlyModifier {
			case "+":
				readOnlyProperties[keyName] = true
			case "":
				if sourceObjectType != nil && sourceObjectType.ReadOnlyProps()[keyName] {
					readOnlyProperties[keyName] = true
				}
			}
		}
	}

	// Create the expanded object type
	return &types.ObjectType{
		Properties:         properties,
		PropertyOrder:      propertyOrder,
		OptionalProperties: optionalProperties,
		ReadOnlyProperties: readOnlyProperties,
		CallSignatures:     []*types.Signature{},      // Mapped types don't create call signatures
		IndexSignatures:    []*types.IndexSignature{}, // TODO: Handle index signatures if needed
	}
}

// substituteTypeParameterInType substitutes a type parameter with a concrete type
// This is used when expanding mapped types to replace P with specific literal types
func (c *Checker) substituteTypeParameterInType(targetType types.Type, paramName string, replacement types.Type) types.Type {
	return c.substituteTypeParameterInTypeMemo(targetType, paramName, replacement, map[*types.ObjectType]*types.ObjectType{})
}

// substituteTypeParameterInTypeMemo is substituteTypeParameterInType with the
// object types already being rewritten: a self-referencing object (a class
// whose method returns Wrap<this>) maps to its in-progress copy instead of
// recursing forever.
func (c *Checker) substituteTypeParameterInTypeMemo(targetType types.Type, paramName string, replacement types.Type, memo map[*types.ObjectType]*types.ObjectType) types.Type {
	if targetType == nil {
		return nil
	}
	recur := func(t types.Type) types.Type {
		return c.substituteTypeParameterInTypeMemo(t, paramName, replacement, memo)
	}

	switch typ := targetType.(type) {
	case *types.TypeParameterType:
		// If this is the type parameter we're looking for, replace it
		if typ.Parameter != nil && typ.Parameter.Name == paramName {
			return replacement
		}
		return targetType

	case *types.ArrayType:
		return &types.ArrayType{
			ElementType: recur(typ.ElementType),
		}

	case *types.TupleType:
		elementTypes := make([]types.Type, len(typ.ElementTypes))
		for i, elementType := range typ.ElementTypes {
			elementTypes[i] = recur(elementType)
		}
		return &types.TupleType{
			ElementTypes:     elementTypes,
			OptionalElements: append([]bool(nil), typ.OptionalElements...),
			RestElementType:  recur(typ.RestElementType),
		}

	case *types.ObjectType:
		if done, ok := memo[typ]; ok {
			return done
		}
		if typ.Pending {
			return typ // filled in later, in place (see substituteTypes)
		}
		if mentions, _ := c.objectMayMention(typ, map[string]types.Type{paramName: replacement}); !mentions {
			// A mapped type's key substituted into `Shape[k]` must not copy
			// the class types inside Shape.
			memo[typ] = typ
			return typ
		}
		return cloneObjectTypeWithTypesNotify(typ, recur, func(clone *types.ObjectType) { memo[typ] = clone })

	case *types.IndexedAccessType:
		// Handle T[P] where P is the type parameter being substituted
		objectType := recur(typ.ObjectType)
		indexType := recur(typ.IndexType)

		// Try to resolve the indexed access with the substituted types
		resolvedType := c.computeIndexedAccessType(objectType, indexType)
		if resolvedType != nil {
			return resolvedType
		}

		// If we can't resolve it, return a new IndexedAccessType with substituted parts
		return &types.IndexedAccessType{
			ObjectType: objectType,
			IndexType:  indexType,
		}

	case *types.UnionType:
		// Recursively substitute in union members
		var substitutedTypes []types.Type
		for _, memberType := range typ.Types {
			substituted := recur(memberType)
			if substituted != nil {
				substitutedTypes = append(substitutedTypes, substituted)
			}
		}
		if len(substitutedTypes) == 0 {
			return nil
		}
		if len(substitutedTypes) == 1 {
			return substitutedTypes[0]
		}
		return types.NewUnionType(substitutedTypes...)

	case *types.ConditionalType, *types.GenericTypeAliasForwardReference, *types.ReadonlyType, *types.IntersectionType:
		// Composite types the cases above don't walk: the general name-based
		// substitution does, evaluating a conditional once its parts are
		// concrete (`T[K] extends object ? DeepReadonly<T[K]> : T[K]`, #613).
		return c.substituteTypes(targetType, map[string]types.Type{paramName: replacement})

	default:
		// For other types (primitives, objects, etc.), no substitution needed
		return targetType
	}
}

func cloneObjectTypeWithTypes(obj *types.ObjectType, rewrite func(types.Type) types.Type) *types.ObjectType {
	return cloneObjectTypeWithTypesNotify(obj, rewrite, nil)
}

// cloneObjectTypeWithTypesNotify is cloneObjectTypeWithTypes that hands the
// clone to created before rewriting any member, so a rewrite that meets obj
// again can use the clone.
func cloneObjectTypeWithTypesNotify(obj *types.ObjectType, rewrite func(types.Type) types.Type, created func(*types.ObjectType)) *types.ObjectType {
	if obj == nil {
		return nil
	}

	result := &types.ObjectType{
		Properties:         make(map[string]types.Type, len(obj.Props())),
		PropertyDocs:       obj.PropertyDocs,
		Doc:                obj.Doc,
		OptionalProperties: make(map[string]bool, len(obj.OptionalProps())),
		ReadOnlyProperties: make(map[string]bool, len(obj.ReadOnlyProps())),
		BaseTypes:          append([]types.Type(nil), obj.Bases()...),
		IsReflectIntrinsic: obj.IsReflectIntrinsic,
	}
	if created != nil {
		created(result)
	}
	result.Materialize()
	result.CallSignatures = cloneSignaturesWithTypes(obj.CallSigs(), rewrite)
	result.Materialize()
	result.ConstructSignatures = cloneSignaturesWithTypes(obj.ConstructSigs(), rewrite)

	for _, name := range obj.PropertyNames() {
		propType := obj.Props()[name]
		result.SetProperty(name, rewrite(propType))
	}
	for name, optional := range obj.OptionalProps() {
		result.OptionalProps()[name] = optional
	}
	for name, readonly := range obj.ReadOnlyProps() {
		result.ReadOnlyProps()[name] = readonly
	}
	if obj.IndexSigs() != nil {
		result.Materialize()
		result.IndexSignatures = make([]*types.IndexSignature, len(obj.IndexSigs()))
		for i, sig := range obj.IndexSigs() {
			if sig == nil {
				continue
			}
			result.IndexSigs()[i] = &types.IndexSignature{
				KeyType:        rewrite(sig.KeyType),
				ValueType:      rewrite(sig.ValueType),
				IsMapped:       sig.IsMapped,
				TypeParameter:  sig.TypeParameter,
				ConstraintType: rewrite(sig.ConstraintType),
			}
		}
	}
	if obj.ClassMeta != nil {
		// A full copy: SourceClassName and the inheritance fields drive
		// protected/private access checks on an instantiated generic class (#613).
		meta := *obj.ClassMeta
		result.ClassMeta = &meta
	}

	return result
}

func cloneSignaturesWithTypes(signatures []*types.Signature, rewrite func(types.Type) types.Type) []*types.Signature {
	if signatures == nil {
		return nil
	}
	result := make([]*types.Signature, len(signatures))
	for i, sig := range signatures {
		if sig == nil {
			continue
		}
		paramTypes := make([]types.Type, len(sig.ParameterTypes))
		for j, paramType := range sig.ParameterTypes {
			paramTypes[j] = rewrite(paramType)
		}
		result[i] = &types.Signature{
			TypeParameters:    sig.TypeParameters,
			ParameterNames:    append([]string(nil), sig.ParameterNames...),
			ParameterTypes:    paramTypes,
			ReturnType:        rewrite(sig.ReturnType),
			OptionalParams:    append([]bool(nil), sig.OptionalParams...),
			IsVariadic:        sig.IsVariadic,
			StrictVariance:    sig.StrictVariance,
			RestParameterType: rewrite(sig.RestParameterType),
		}
	}
	return result
}

// isAssignableWithExpansion checks if source can be assigned to target,
// but first expands any mapped types to concrete object types
func (c *Checker) isAssignableWithExpansion(source, target types.Type) bool {
	debugPrintf("// [Checker] isAssignableWithExpansion: source=%T target=%T\n", source, target)
	if checkerDebug {
		debugPrintf("// [Checker] source: %s\n", source.String())
	}
	if checkerDebug {
		debugPrintf("// [Checker] target: %s\n", target.String())
	}

	// Resolve typeof forward references before assignability checking
	source = c.resolveTypeofTypeIfNeeded(source)
	target = c.resolveTypeofTypeIfNeeded(target)

	// Expand target if it's a mapped type or instantiated type containing a mapped type
	expandedTarget := c.expandIfMappedType(target)

	// Expand source if it's a mapped type (less common but possible)
	expandedSource := c.expandIfMappedType(source)

	// Use the standard assignability check with expanded types
	result := types.IsAssignable(expandedSource, expandedTarget)
	debugPrintf("// [Checker] isAssignableWithExpansion result: %v\n", result)
	return result
}

// isArgumentAssignableWithExpansion is like isAssignableWithExpansion, but
// used specifically for checking a value passed as a call argument against
// its parameter type. It additionally tolerates a `void`-returning function
// argument wherever the parameter's declared type is itself a function type
// (see types.IsAssignableForCallArgument for why this is scoped to call
// arguments rather than general assignability).
func (c *Checker) isArgumentAssignableWithExpansion(source, target types.Type) bool {
	source = c.resolveTypeofTypeIfNeeded(source)
	target = c.resolveTypeofTypeIfNeeded(target)

	expandedTarget := c.expandIfMappedType(target)
	expandedSource := c.expandIfMappedType(source)

	return types.IsAssignableForCallArgument(expandedSource, expandedTarget)
}

// expandIfMappedType expands a type if it's a mapped type or contains a mapped type
func (c *Checker) expandIfMappedType(typ types.Type) types.Type {
	if typ == nil {
		return typ
	}
	typ = c.settleType(typ)

	if checkerDebug {
		debugPrintf("// [Checker] expandIfMappedType called with type: %T %s\n", typ, typ.String())
	}

	// Direct mapped type
	if mappedType, ok := typ.(*types.MappedType); ok {
		if checkerDebug {
			debugPrintf("// [Checker] Found direct mapped type: %s\n", mappedType.String())
		}
		if checkerDebug {
			debugPrintf("// [Checker] Constraint: %T %s\n", mappedType.ConstraintType, mappedType.ConstraintType.String())
		}
		if checkerDebug {
			debugPrintf("// [Checker] ValueType: %T %s\n", mappedType.ValueType, mappedType.ValueType.String())
		}
		expanded := c.expandMappedType(mappedType)
		if expanded != nil {
			if checkerDebug {
				debugPrintf("// [Checker] Direct mapped type expanded to: %s\n", expanded.String())
			}
			return c.settleType(expanded)
		}
		debugPrintf("// [Checker] Direct mapped type expansion failed\n")
		return typ
	}

	// Union members (`Partial<T> | undefined` for an optional parameter)
	if union, ok := typ.(*types.UnionType); ok {
		changed := false
		members := make([]types.Type, len(union.Types))
		for i, member := range union.Types {
			members[i] = c.expandIfMappedType(member)
			if members[i] != member {
				changed = true
			}
		}
		if changed {
			return types.NewUnionType(members...)
		}
		return typ
	}

	// Intersection members (zod's addQuestionMarks<T> is three mapped types
	// intersected)
	if inter, ok := typ.(*types.IntersectionType); ok {
		changed := false
		members := make([]types.Type, len(inter.Types))
		for i, member := range inter.Types {
			members[i] = c.expandIfMappedType(member)
			if members[i] != member {
				changed = true
			}
		}
		if changed {
			return types.NewIntersectionType(members...)
		}
		return typ
	}

	// Instantiated type that might contain a mapped type
	if instantiated, ok := typ.(*types.InstantiatedType); ok {
		debugPrintf("// [Checker] Found InstantiatedType, checking body...\n")
		// Check if the instantiated type's body is a mapped type
		if instantiated.Generic != nil && instantiated.Generic.Body != nil {
			if checkerDebug {
				debugPrintf("// [Checker] InstantiatedType body: %T %s\n", instantiated.Generic.Body, instantiated.Generic.Body.String())
			}
			if mappedType, ok := instantiated.Generic.Body.(*types.MappedType); ok {
				debugPrintf("// [Checker] InstantiatedType contains mapped type, substituting...\n")
				// We need to substitute the type arguments in the mapped type
				substitutedMappedType := c.substituteMappedType(mappedType, instantiated.Generic.TypeParameters, instantiated.TypeArguments)
				if substitutedMappedType != nil {
					if checkerDebug {
						debugPrintf("// [Checker] Substituted mapped type: %s\n", substitutedMappedType.String())
					}
					expanded := c.expandMappedType(substitutedMappedType)
					if expanded != nil {
						if checkerDebug {
							debugPrintf("// [Checker] InstantiatedType expanded to: %s\n", expanded.String())
						}
						return expanded
					}
				}
			}
		}
	}

	debugPrintf("// [Checker] No expansion performed, returning original type\n")
	return typ
}

// maxInferDepth bounds the structural walk of tryInferTypes for types that
// keep expanding into fresh instantiations instead of revisiting a pair.
const maxInferDepth = 100

// inferPair is a (source, target) pair being matched by tryInferTypes.
type inferPair struct{ check, extends types.Type }

// inferFromConditional runs one top-level infer match (one conditional type)
// with its own visited set, so a nested conditional resolved mid-walk can't
// mistake the outer walk's pairs for its own cycles.
func (c *Checker) inferFromConditional(checkType, extendsType types.Type, inferences map[string]types.Type) bool {
	savedVisiting, savedDepth := c.inferVisiting, c.inferDepth
	c.inferVisiting, c.inferDepth = nil, 0
	defer func() { c.inferVisiting, c.inferDepth = savedVisiting, savedDepth }()
	return c.tryInferTypes(checkType, extendsType, inferences)
}

// tryInferTypes attempts to match checkType against extendsType and capture infer types
func (c *Checker) tryInferTypes(checkType, extendsType types.Type, inferences map[string]types.Type) bool {
	// Recursive types (Observable.pipe(): Observable<...>) bring the walk back
	// to a pair it is still matching. Like TypeScript's inference `visited`
	// map, a pair already in progress contributes nothing further and is
	// assumed to match; the depth cap catches ever-growing instantiations.
	if checkType == nil || extendsType == nil {
		return false
	}
	pair := inferPair{checkType, extendsType}
	if c.inferVisiting[pair] || c.inferDepth >= maxInferDepth {
		return true
	}
	if c.inferVisiting == nil {
		c.inferVisiting = make(map[inferPair]bool)
	}
	c.inferVisiting[pair] = true
	c.inferDepth++
	defer func() {
		delete(c.inferVisiting, pair)
		c.inferDepth--
	}()

	if checkerDebug {
		debugPrintf("// [TryInfer] Trying to infer from %s to %s\n", checkType.String(), extendsType.String())
	}
	switch et := extendsType.(type) {
	case *types.InferType:
		// This is an infer type - capture the checkType
		// If we already have an inference for this parameter, create a union
		if existingType, found := inferences[et.TypeParameter]; found {
			// Create union of existing and new inferred types
			inferences[et.TypeParameter] = types.NewUnionType(existingType, checkType)
		} else {
			inferences[et.TypeParameter] = checkType
		}
		return true

	case *types.ObjectType:
		// Handle function type inference like (...args: any[]) => infer R
		debugPrintf("// [TryInfer] Both are ObjectType\n")
		if ct, ok := checkType.(*types.ObjectType); ok {
			if checkerDebug {
				debugPrintf("// [TryInfer] Check has %d call sigs, %d construct sigs\n", len(ct.CallSigs()), len(ct.ConstructSigs()))
			}
			if checkerDebug {
				debugPrintf("// [TryInfer] Extends has %d call sigs, %d construct sigs\n", len(et.CallSigs()), len(et.ConstructSigs()))
			}
			// Try call signatures first
			if len(ct.CallSigs()) > 0 && len(et.CallSigs()) > 0 {
				// Match function signature structure (use first call signature)
				debugPrintf("// [TryInfer] Trying call signature inference\n")
				return c.tryInferFromFunctionTypes(ct.CallSigs()[0], et.CallSigs()[0], inferences)
			}
			// Try constructor signatures for ConstructorParameters<T> and InstanceType<T>
			if len(ct.ConstructSigs()) > 0 && len(et.ConstructSigs()) > 0 {
				// Match constructor signature structure (use first construct signature)
				debugPrintf("// [TryInfer] Trying constructor signature inference\n")
				return c.tryInferFromFunctionTypes(ct.ConstructSigs()[0], et.ConstructSigs()[0], inferences)
			}
			// Plain object types: infer property by property, e.g.
			// { a: infer U, b: infer U } against { a: string, b: number }.
			if len(et.Props()) > 0 && len(ct.CallSigs()) == 0 && len(ct.ConstructSigs()) == 0 {
				names := make([]string, 0, len(et.Props()))
				for name := range et.Props() {
					names = append(names, name)
				}
				sort.Strings(names)
				for _, name := range names {
					checkProp, ok := ct.Props()[name]
					if !ok {
						return false
					}
					if !c.tryInferTypes(checkProp, et.Props()[name], inferences) {
						return false
					}
				}
				return true
			}
		}
		debugPrintf("// [TryInfer] ObjectType inference failed\n")
		return false

	case *types.ArrayType:
		// Handle array type inference like (infer U)[]
		if ct, ok := checkType.(*types.ArrayType); ok {
			return c.tryInferTypes(ct.ElementType, et.ElementType, inferences)
		}
		// Handle tuple to array inference like [string] to any[]
		if ct, ok := checkType.(*types.TupleType); ok {
			debugPrintf("// [TryInfer] Tuple to array: checking if all tuple elements match array element type\n")
			// All tuple elements must be assignable to the array element type
			for i, tupleElementType := range ct.ElementTypes {
				if checkerDebug {
					debugPrintf("// [TryInfer] Checking tuple element %d: %s -> %s\n", i, tupleElementType.String(), et.ElementType.String())
				}
				if !c.tryInferTypes(tupleElementType, et.ElementType, inferences) {
					debugPrintf("// [TryInfer] Tuple element %d failed inference\n", i)
					return false
				}
			}
			debugPrintf("// [TryInfer] All tuple elements matched array element type\n")
			return true
		}
		return false

	default:
		// For other types, check if they're structurally compatible
		return types.IsAssignable(checkType, extendsType)
	}
}

// tryInferFromFunctionTypes handles function type inference
func (c *Checker) tryInferFromFunctionTypes(checkSig, extendsSig *types.Signature, inferences map[string]types.Type) bool {
	debugPrintf("// [TryInferFunc] === Function inference ===\n")

	// Add nil checks to prevent panic
	if checkSig == nil || extendsSig == nil {
		debugPrintf("// [TryInferFunc] One of the signatures is nil\n")
		return false
	}

	// Check for nil return types
	if checkSig.ReturnType == nil {
		debugPrintf("// [TryInferFunc] Check return type is nil\n")
		return false
	}
	if extendsSig.ReturnType == nil {
		debugPrintf("// [TryInferFunc] Extends return type is nil\n")
		return false
	}

	if checkerDebug {
		debugPrintf("// [TryInferFunc] Check return: %s\n", checkSig.ReturnType.String())
	}
	if checkerDebug {
		debugPrintf("// [TryInferFunc] Extends return: %s\n", extendsSig.ReturnType.String())
	}
	// Try to infer from return type
	if !c.tryInferTypes(checkSig.ReturnType, extendsSig.ReturnType, inferences) {
		debugPrintf("// [TryInferFunc] Return type inference failed\n")
		return false
	}
	debugPrintf("// [TryInferFunc] Return type inference succeeded\n")

	// Try to infer from parameter types
	if checkerDebug {
		debugPrintf("// [TryInferFunc] Check params: %d, Extends params: %d\n", len(checkSig.ParameterTypes), len(extendsSig.ParameterTypes))
	}
	minParams := len(checkSig.ParameterTypes)
	if len(extendsSig.ParameterTypes) < minParams {
		minParams = len(extendsSig.ParameterTypes)
	}

	for i := 0; i < minParams; i++ {
		// Add nil checks for parameter types
		if checkSig.ParameterTypes[i] == nil || extendsSig.ParameterTypes[i] == nil {
			debugPrintf("// [TryInferFunc] Parameter %d type is nil\n", i)
			return false
		}
		if checkerDebug {
			debugPrintf("// [TryInferFunc] Inferring param %d: %s -> %s\n", i, checkSig.ParameterTypes[i].String(), extendsSig.ParameterTypes[i].String())
		}
		if !c.tryInferTypes(checkSig.ParameterTypes[i], extendsSig.ParameterTypes[i], inferences) {
			debugPrintf("// [TryInferFunc] Parameter %d inference failed\n", i)
			return false
		}
	}
	debugPrintf("// [TryInferFunc] All parameters inferred successfully\n")

	// NEW: Handle rest parameter inference
	// This is crucial for Parameters<T> utility type
	if extendsSig.RestParameterType != nil {
		if checkerDebug {
			debugPrintf("// [TryInferFunc] Rest parameter inference needed. Extends rest type: %s\n", extendsSig.RestParameterType.String())
		}
		// For Parameters<T>, we want to infer the parameter types as a tuple
		// The extends pattern is (...args: infer P) => any where P should be the parameter tuple

		// Create a tuple from ALL function parameters (both regular and rest)
		var allParamTypes []types.Type
		var optionalElements []bool
		var restElementType types.Type

		// Add all regular parameters first
		allParamTypes = append(allParamTypes, checkSig.ParameterTypes...)

		// Mark optional parameters
		if checkSig.OptionalParams != nil {
			// Extend optionalElements to match the length of allParamTypes
			optionalElements = make([]bool, len(allParamTypes))
			copy(optionalElements, checkSig.OptionalParams)
		} else {
			optionalElements = make([]bool, len(allParamTypes))
		}

		// If there's a rest parameter, handle it as a rest element type
		if checkSig.RestParameterType != nil {
			// For Parameters<T>, the rest parameter type (e.g., number[])
			// becomes the rest element type in the tuple
			// So f(...rest: number[]) -> [...number[]]
			restElementType = checkSig.RestParameterType
		}

		// Create tuple type from all parameters
		tupleType := &types.TupleType{
			ElementTypes:     allParamTypes,
			OptionalElements: optionalElements,
			RestElementType:  restElementType,
		}
		if checkerDebug {
			debugPrintf("// [TryInferFunc] Created tuple: %s\n", tupleType.String())
		}

		if !c.tryInferTypes(tupleType, extendsSig.RestParameterType, inferences) {
			debugPrintf("// [TryInferFunc] Rest parameter inference failed\n")
			return false
		}
	}

	return true
}

// substituteInferredTypes substitutes inferred types into the target type
func (c *Checker) substituteInferredTypes(targetType types.Type, inferences map[string]types.Type) types.Type {
	if targetType == nil {
		return nil
	}

	switch t := targetType.(type) {
	case *types.InferType:
		// Replace infer type with inferred type
		if inferredType, found := inferences[t.TypeParameter]; found {
			return inferredType
		}
		return targetType

	case *types.ArrayType:
		// Recursively substitute in array element type
		substitutedElement := c.substituteInferredTypes(t.ElementType, inferences)
		return &types.ArrayType{ElementType: substitutedElement}

	case *types.UnionType:
		// Recursively substitute in union members
		var substitutedTypes []types.Type
		for _, memberType := range t.Types {
			substituted := c.substituteInferredTypes(memberType, inferences)
			substitutedTypes = append(substitutedTypes, substituted)
		}
		return types.NewUnionType(substitutedTypes...)

	default:
		// For other types, return as-is
		return targetType
	}
}

// substituteMappedType substitutes type arguments into a mapped type
func (c *Checker) substituteMappedType(mappedType *types.MappedType, typeParams []*types.TypeParameter, typeArgs []types.Type) *types.MappedType {
	if mappedType == nil || len(typeParams) != len(typeArgs) {
		return mappedType
	}

	// Create substitution map
	substitutions := make(map[string]types.Type)
	for i, param := range typeParams {
		if i < len(typeArgs) {
			substitutions[param.Name] = typeArgs[i]
		}
	}

	// Substitute in constraint type
	substitutedConstraint := c.substituteInType(mappedType.ConstraintType, substitutions)

	// Substitute in value type
	substitutedValue := c.substituteInType(mappedType.ValueType, substitutions)

	return &types.MappedType{
		TypeParameter:    mappedType.TypeParameter,
		ConstraintType:   substitutedConstraint,
		ValueType:        substitutedValue,
		ReadonlyModifier: mappedType.ReadonlyModifier,
		OptionalModifier: mappedType.OptionalModifier,
	}
}

// evaluateConditionalType evaluates a conditional type after substitution.
// It distributes over union check types (TypeScript's distributive conditional types).
// For example, Exclude<"a" | "b" | "c", "b"> distributes as:
//
//	("a" extends "b" ? never : "a") | ("b" extends "b" ? never : "b") | ("c" extends "b" ? never : "c")
//	= "a" | never | "c" = "a" | "c"
func (c *Checker) evaluateConditionalType(checkType, extendsType, trueType, falseType types.Type) types.Type {
	// Handle KeyofType in checkType: expand to union of literal keys
	if keyofType, ok := checkType.(*types.KeyofType); ok {
		if objType, ok := keyofType.OperandType.(*types.ObjectType); ok {
			var keyTypes []types.Type
			for propName := range objType.Props() {
				keyTypes = append(keyTypes, &types.LiteralType{Value: vm.String(propName)})
			}
			if len(keyTypes) > 0 {
				checkType = types.NewUnionType(keyTypes...)
			}
		}
	}

	// Distributive conditional types: distribute over union check types
	if unionCheck, ok := checkType.(*types.UnionType); ok {
		var resultTypes []types.Type
		for _, member := range unionCheck.Types {
			result := c.computeConditionalType(member, extendsType, trueType, falseType)
			if result == nil {
				return &types.ConditionalType{CheckType: checkType, ExtendsType: extendsType, TrueType: trueType, FalseType: falseType}
			}
			if result != types.Never {
				resultTypes = append(resultTypes, result)
			}
		}
		if len(resultTypes) == 0 {
			return types.Never
		}
		return types.NewUnionType(resultTypes...)
	}

	// Non-union: evaluate directly; a conditional that cannot be decided
	// yet stays one.
	if result := c.computeConditionalType(checkType, extendsType, trueType, falseType); result != nil {
		return result
	}
	return &types.ConditionalType{CheckType: checkType, ExtendsType: extendsType, TrueType: trueType, FalseType: falseType}
}

// substituteInType performs type substitution based on a substitution map
func (c *Checker) substituteInType(typ types.Type, substitutions map[string]types.Type) types.Type {
	if typ == nil {
		return nil
	}

	switch t := typ.(type) {
	case *types.TypeParameterType:
		if t.Parameter != nil {
			if replacement, exists := substitutions[t.Parameter.Name]; exists {
				return replacement
			}
		}
		return typ

	case *types.ArrayType:
		return &types.ArrayType{
			ElementType: c.substituteInType(t.ElementType, substitutions),
		}

	case *types.TupleType:
		elementTypes := make([]types.Type, len(t.ElementTypes))
		for i, elementType := range t.ElementTypes {
			elementTypes[i] = c.substituteInType(elementType, substitutions)
		}
		return &types.TupleType{
			ElementTypes:     elementTypes,
			OptionalElements: append([]bool(nil), t.OptionalElements...),
			RestElementType:  c.substituteInType(t.RestElementType, substitutions),
		}

	case *types.ObjectType:
		return cloneObjectTypeWithTypes(t, func(member types.Type) types.Type {
			return c.substituteInType(member, substitutions)
		})

	case *types.KeyofType:
		substitutedOperand := c.substituteInType(t.OperandType, substitutions)
		return &types.KeyofType{OperandType: substitutedOperand}

	case *types.IndexedAccessType:
		substitutedObject := c.substituteInType(t.ObjectType, substitutions)
		substitutedIndex := c.substituteInType(t.IndexType, substitutions)
		return &types.IndexedAccessType{
			ObjectType: substitutedObject,
			IndexType:  substitutedIndex,
		}

	case *types.ConditionalType:
		substitutedCheck := c.substituteInType(t.CheckType, substitutions)
		substitutedExtends := c.substituteInType(t.ExtendsType, substitutions)
		substitutedTrue := c.substituteInType(t.TrueType, substitutions)
		substitutedFalse := c.substituteInType(t.FalseType, substitutions)
		// After substitution, try to evaluate the conditional type
		// This handles Exclude<keyof T, K> by distributing over the union
		return c.evaluateConditionalType(substitutedCheck, substitutedExtends, substitutedTrue, substitutedFalse)

	case *types.UnionType:
		substitutedMembers := make([]types.Type, len(t.Types))
		for i, member := range t.Types {
			substitutedMembers[i] = c.substituteInType(member, substitutions)
		}
		return types.NewUnionType(substitutedMembers...)

	default:
		return typ
	}
}

// resolveTemplateLiteralTypeExpression resolves a template literal type expression to a TemplateLiteralType
func (c *Checker) resolveTemplateLiteralTypeExpression(node *parser.TemplateLiteralTypeExpression) types.Type {
	if node == nil || len(node.Parts) == 0 {
		c.addError(node, "empty template literal type")
		return nil
	}

	var parts []types.TemplateLiteralPart

	for i, part := range node.Parts {
		if i%2 == 0 {
			// Even indices are string parts
			if stringPart, ok := part.(*parser.TemplateStringPart); ok {
				parts = append(parts, types.TemplateLiteralPart{
					IsLiteral: true,
					Literal:   stringPart.Value,
					Type:      nil,
				})
			} else {
				c.addError(node, "expected string part in template literal type")
				return nil
			}
		} else {
			// Odd indices are type expressions
			if typeExpr, ok := part.(parser.Expression); ok {
				resolvedType := c.resolveTypeAnnotation(typeExpr)
				if resolvedType == nil {
					// Error already reported by resolveTypeAnnotation
					return nil
				}
				parts = append(parts, types.TemplateLiteralPart{
					IsLiteral: false,
					Literal:   "",
					Type:      resolvedType,
				})
			} else {
				c.addError(node, "expected type expression in template literal type interpolation")
				return nil
			}
		}
	}

	// Try to compute the template literal type to a concrete string literal
	computedType := c.computeTemplateLiteralType(&types.TemplateLiteralType{Parts: parts})
	if computedType != nil {
		return computedType
	}

	return &types.TemplateLiteralType{
		Parts: parts,
	}
}

// substituteTypesPreservingInfer performs type substitution but preserves InferType nodes
func (c *Checker) substituteTypesPreservingInfer(typ types.Type, substitution map[string]types.Type) types.Type {
	return c.substituteTypesPreservingInferVisited(typ, substitution, make(map[types.Type]types.Type))
}

// substituteTypesPreservingInferVisited is substituteTypesPreservingInfer
// sharing the caller's substitution memo, so one instantiation copies each
// shared part once.
func (c *Checker) substituteTypesPreservingInferVisited(typ types.Type, substitution map[string]types.Type, visited map[types.Type]types.Type) types.Type {
	if typ == nil {
		return nil
	}

	switch t := typ.(type) {
	case *types.InferType:
		// Don't substitute infer types - they should remain as inference placeholders
		return typ

	case *types.TypeParameterType:
		// Substitute type parameters as normal
		if t.Parameter != nil {
			if replacement, found := substitution[t.Parameter.Name]; found {
				return replacement
			}
		}
		return typ

	case *types.ArrayType:
		// Recursively preserve infer in array element type
		substitutedElement := c.substituteTypesPreservingInferVisited(t.ElementType, substitution, visited)
		return &types.ArrayType{ElementType: substitutedElement}

	case *types.UnionType:
		// Recursively preserve infer in union members
		var substitutedTypes []types.Type
		for _, memberType := range t.Types {
			substituted := c.substituteTypesPreservingInferVisited(memberType, substitution, visited)
			substitutedTypes = append(substitutedTypes, substituted)
		}
		return types.NewUnionType(substitutedTypes...)

	case *types.ObjectType:
		// For function types, recursively handle call signatures
		if len(t.CallSigs()) > 0 {
			newSigs := make([]*types.Signature, len(t.CallSigs()))
			for i, sig := range t.CallSigs() {
				// Substitute in signature parameters and return type while preserving infer
				newParamTypes := make([]types.Type, len(sig.ParameterTypes))
				for j, paramType := range sig.ParameterTypes {
					newParamTypes[j] = c.substituteTypesPreservingInferVisited(paramType, substitution, visited)
				}
				newReturnType := c.substituteTypesPreservingInferVisited(sig.ReturnType, substitution, visited)

				newSigs[i] = &types.Signature{
					TypeParameters:    sig.TypeParameters,
					ParameterNames:    sig.ParameterNames,
					ParameterTypes:    newParamTypes,
					ReturnType:        newReturnType,
					OptionalParams:    sig.OptionalParams,
					IsVariadic:        sig.IsVariadic,
					StrictVariance:    sig.StrictVariance,
					RestParameterType: sig.RestParameterType,
				}
			}

			// Create new ObjectType with substituted signatures
			newObj := types.NewObjectType()
			for _, sig := range newSigs {
				newObj.WithCallSignature(sig)
			}
			return newObj
		}
		return typ

	default:
		// For other types, fall back to regular substitution
		return c.substituteTypesWithVisited(typ, substitution, visited)
	}
}

// computeTemplateLiteralType attempts to compute a template literal type to a concrete string literal
// Returns nil if the template contains non-literal types that can't be computed
func (c *Checker) computeTemplateLiteralType(tlt *types.TemplateLiteralType) types.Type {
	var result strings.Builder

	for _, part := range tlt.Parts {
		if part.IsLiteral {
			// String literal part - add directly
			result.WriteString(part.Literal)
		} else {
			// Type interpolation part - try to extract string literal
			if literalType, ok := part.Type.(*types.LiteralType); ok {
				if literalType.Value.Type() == vm.TypeString {
					// It's a string literal - add its value
					result.WriteString(literalType.Value.AsString())
				} else {
					// Non-string literal - can't compute
					return nil
				}
			} else {
				// Non-literal type - can't compute to concrete string
				return nil
			}
		}
	}

	// All parts were computable - return as string literal type
	computedValue := result.String()
	return &types.LiteralType{
		Value: vm.String(computedValue),
	}
}

// resolveEnumMemberTypeExpression resolves qualified type access in type context:
// 'Color.Red', 'N.MyType', or arbitrarily deep 'A.B.C.D'.
// node.Object may be an *Identifier (simple name) or another *MemberExpression (chained).
func (c *Checker) resolveEnumMemberTypeExpression(node *parser.MemberExpression) types.Type {
	if checkerDebug {
		debugPrintf("// [Checker resolveEnumMemberType] Resolving %s.%s\n",
			node.Object.String(), node.Property.String())
	}

	memberIdent, ok := node.Property.(*parser.Identifier)
	if !ok {
		c.addError(node.Property, "enum member access requires identifier after '.'")
		return nil
	}
	memberName := memberIdent.Value

	// Resolve the object side to get the container type.
	var containerType types.Type
	switch obj := node.Object.(type) {
	case *parser.Identifier:
		// Prefer the type env for NamespaceType. Enums live in the value env as
		// *types.EnumType (their type-env entry is the union of members, not EnumType).
		if t, found := c.env.ResolveType(obj.Value); found {
			if _, isNs := t.(*types.NamespaceType); isNs {
				containerType = t
			}
		}
		if containerType == nil {
			if t, _, found := c.env.Resolve(obj.Value); found {
				containerType = t
			} else {
				c.addCannotFindNameError(node.Object, c.env, obj.Value)
				return nil
			}
		}
	case *parser.MemberExpression:
		containerType = c.resolveEnumMemberTypeExpression(obj)
		if containerType == nil {
			return nil
		}
	case *parser.ImportTypeExpression:
		ns := c.importTypeNamespace(obj)
		if ns == nil {
			return types.Any
		}
		containerType = ns
	default:
		c.addError(node.Object, "enum member access requires identifier before '.'")
		return nil
	}

	switch ct := containerType.(type) {
	case *types.NamespaceType:
		if member := ct.LookupTypeMember(memberName); member != nil {
			return member
		}
		if obj, ok := node.Object.(*parser.Identifier); ok && c.isLibGlobalValue(obj.Value) {
			// A lib namespace (Intl, ...) whose member Paserati does not model.
			return types.Any
		}
		c.addErrorWithCode(node.Property, errors.TS2694, fmt.Sprintf("Namespace '%s' has no exported member '%s'.", node.Object.String(), memberName))
		return nil
	case *types.EnumType:
		memberType, exists := ct.Members[memberName]
		if !exists {
			c.addError(node.Property, fmt.Sprintf("property '%s' does not exist on enum %s", memberName, node.Object.String()))
			return nil
		}
		if checkerDebug {
			debugPrintf("// [Checker resolveEnumMemberType] Resolved %s.%s to %s\n",
				node.Object.String(), memberName, memberType.String())
		}
		return memberType
	default:
		c.addError(node.Object, fmt.Sprintf("'%s' is not a namespace or enum type", node.Object.String()))
		return nil
	}
}

// overloadableCallSignatures returns the call signatures of a method type that
// can take part in an overload set (a plain callable object or a generic
// function), with a generic function's type parameters recorded on the
// signature itself.
func overloadableCallSignatures(t types.Type) ([]*types.Signature, bool) {
	switch tt := t.(type) {
	case *types.ObjectType:
		if tt.IsCallable() && len(tt.Props()) == 0 && len(tt.ConstructSigs()) == 0 {
			return tt.GetCallSignatures(), true
		}
	case *types.GenericType:
		if body, ok := tt.Body.(*types.ObjectType); ok && body.IsCallable() && len(body.Props()) == 0 && len(body.ConstructSigs()) == 0 {
			var out []*types.Signature
			for _, sig := range body.GetCallSignatures() {
				cp := *sig
				cp.TypeParameters = append([]*types.TypeParameter(nil), tt.TypeParameters...)
				out = append(out, &cp)
			}
			return out, true
		}
	}
	return nil, false
}

// isNumericIndexType reports whether t is `number` or a number literal.
func isNumericIndexType(t types.Type) bool {
	if t == types.Number {
		return true
	}
	lit, ok := t.(*types.LiteralType)
	return ok && (lit.Value.Type() == vm.TypeIntegerNumber || lit.Value.Type() == vm.TypeFloatNumber)
}

// walkTypeofPath follows the members of a `typeof a.b.c` path through the type
// of its first segment, reporting at node when a member is missing.
func (c *Checker) walkTypeofPath(node parser.Node, path []string, varType types.Type) (types.Type, bool) {
	for i := 1; i < len(path); i++ {
		seg := path[i]
		// A generic class's constructor is a GenericType; its static members live on the body.
		if generic, ok := varType.(*types.GenericType); ok {
			if body, isObj := generic.Body.(*types.ObjectType); isObj {
				varType = body
			}
		}
		switch t := varType.(type) {
		case *types.ObjectType:
			if prop, ok := t.Props()[seg]; ok {
				varType = prop
			} else {
				c.addError(node, fmt.Sprintf("property '%s' does not exist on type '%s'", seg, t.String()))
				return nil, false
			}
		case *types.NamespaceType:
			if prop, ok := t.ValueShape.Props()[seg]; ok {
				varType = prop
			} else if member := t.LookupTypeMember(seg); member != nil {
				varType = member
			} else {
				c.addErrorWithCode(node, errors.TS2694, fmt.Sprintf("Namespace '%s' has no exported member '%s'.", path[i-1], seg))
				return nil, false
			}
		default:
			if varType == types.Any {
				continue // `typeof anyValue.member` is any
			}
			c.addError(node, fmt.Sprintf("cannot access member '%s' on non-object type", seg))
			return nil, false
		}
	}

	return varType, true
}

// speculativeTypeofPath walks a deferred dotted typeof path without reporting;
// ok is false while a member is not known yet.
func (c *Checker) speculativeTypeofPath(path []string, varType types.Type) (result types.Type, ok bool) {
	ok = c.speculate(func() {
		result, _ = c.walkTypeofPath(&parser.Identifier{Token: &lexer.Token{}, Value: path[0]}, path, varType)
	})
	if result == nil {
		ok = false
	}
	return result, ok
}

// inferAsAny replaces `infer X` in a conditional's true branch with any.
func (c *Checker) inferAsAny(t types.Type) types.Type {
	switch tt := t.(type) {
	case *types.InferType:
		return types.Any
	case *types.ArrayType:
		return &types.ArrayType{ElementType: c.inferAsAny(tt.ElementType)}
	case *types.UnionType:
		members := make([]types.Type, len(tt.Types))
		for i, m := range tt.Types {
			members[i] = c.inferAsAny(m)
		}
		return types.NewUnionType(members...)
	}
	return t
}

// removeUndefinedMember drops `undefined` from a union (`-?` modifier).
func removeUndefinedMember(t types.Type) types.Type {
	union, ok := t.(*types.UnionType)
	if !ok {
		return t
	}
	var kept []types.Type
	for _, m := range union.Types {
		if m != types.Undefined {
			kept = append(kept, m)
		}
	}
	if len(kept) == len(union.Types) || len(kept) == 0 {
		return t
	}
	return types.NewUnionType(kept...)
}

// structuralObject reduces an object type, a mapped type that expands to one,
// or an intersection of those to a single object type, so a homomorphic
// mapped type over it (zod's flatten<T> = { [k in keyof T]: T[k] }) sees
// every property with its modifiers. An intersected property has the
// intersection of the member types and is optional only when every member
// that has it marks it optional.
func (c *Checker) structuralObject(t types.Type) (*types.ObjectType, bool) {
	switch tt := t.(type) {
	case *types.ObjectType:
		return tt, true
	case *types.MappedType:
		if expanded, ok := c.expandMappedType(tt).(*types.ObjectType); ok {
			return expanded, true
		}
	case *types.IntersectionType:
		merged := types.NewObjectType()
		optional := map[string]bool{}
		readonly := map[string]bool{}
		for _, member := range tt.Types {
			obj, ok := c.structuralObject(member)
			if !ok {
				return nil, false
			}
			for _, name := range obj.PropertyNames() {
				prop := obj.Props()[name]
				if prev, exists := merged.Props()[name]; exists {
					merged.SetProperty(name, types.NewIntersectionType(prev, prop))
					optional[name] = optional[name] && obj.OptionalProps()[name]
					readonly[name] = readonly[name] && obj.ReadOnlyProps()[name]
				} else {
					merged.SetProperty(name, prop)
					optional[name] = obj.OptionalProps()[name]
					readonly[name] = obj.ReadOnlyProps()[name]
				}
			}
			merged.Materialize()
			merged.IndexSignatures = append(merged.IndexSigs(), obj.IndexSigs()...)
			merged.Materialize()
			merged.CallSignatures = append(merged.CallSigs(), obj.CallSigs()...)
		}
		for name, opt := range optional {
			if opt {
				merged.OptionalProps()[name] = true
			}
		}
		for name, ro := range readonly {
			if ro {
				if merged.ReadOnlyProps() == nil {
					merged.Materialize()
					merged.ReadOnlyProperties = map[string]bool{}
				}
				merged.ReadOnlyProps()[name] = true
			}
		}
		return merged, true
	}
	return nil, false
}

// lookupTypeNoPending is ResolveType without side effects: nil when name is
// not (yet) a type.
func (c *Checker) lookupTypeNoPending(name string) types.Type {
	if t, ok := c.env.ResolveType(name); ok {
		return t
	}
	return nil
}

type genericInstKey struct {
	generic *types.GenericType
	args    string
}

// instantiationCache holds the instantiations of generics whose declarations
// are complete. The checkers of one program share it (a generic declared in
// one module is instantiated by every module importing it), so an entry is
// valid only while its generic's body is the one it was made from.
type instantiationCache struct {
	complete map[*types.GenericType]bool
	inst     map[genericInstKey]types.Type
	closed   map[*types.GenericType]bool // see genericClosed
}

func newInstantiationCache() *instantiationCache {
	return &instantiationCache{
		complete: map[*types.GenericType]bool{},
		inst:     map[genericInstKey]types.Type{},
		closed:   map[*types.GenericType]bool{},
	}
}

// reopen forgets g's instantiations: a declaration merging into g is about
// to change its body in place.
func (ic *instantiationCache) reopen(g *types.GenericType) {
	if !ic.complete[g] {
		return
	}
	delete(ic.complete, g)
	delete(ic.closed, g)
	for key := range ic.inst {
		if key.generic == g {
			delete(ic.inst, key)
		}
	}
}

// markGenericComplete records that the generic declared as name (type and,
// for a class, constructor) has its final body.
func (c *Checker) markGenericComplete(name string) {
	if g, ok := c.lookupTypeNoPending(name).(*types.GenericType); ok {
		c.instCache.complete[g] = true
	}
	if v, _, ok := c.env.Resolve(name); ok {
		if g, isGeneric := v.(*types.GenericType); isGeneric {
			c.instCache.complete[g] = true
		}
	}
}

// objectMayMention is mayMentionTypeParamNames with a visited set reused
// across calls (substitution asks once per object it meets). When it reports
// no mention, seen holds every object the walk entered, none of which
// mentions names either; it is valid until the next call.
func (c *Checker) objectMayMention(t types.Type, names map[string]types.Type) (mentions bool, seen map[types.Type]bool) {
	// Reading a lazily substituted object's members mid-walk substitutes,
	// which asks again: a nested call takes a set of its own instead of
	// clearing the one this walk is using.
	seen, shadowed := c.mentionScratch, c.mentionShadowed
	c.mentionScratch, c.mentionShadowed = nil, nil
	if seen == nil {
		seen, shadowed = make(map[types.Type]bool), make(map[string]map[types.Type]bool)
	} else {
		clear(seen)
		clear(shadowed)
	}
	mentions = mayMentionTypeParamNames(t, names, seen, shadowed)
	c.mentionScratch, c.mentionShadowed = seen, shadowed
	return mentions, seen
}

// mayMentionTypeParamNames reports whether t can refer to a type parameter
// named in names. It is conservative: a kind of type it does not walk counts
// as a mention, so a false result means substitution leaves t unchanged. An
// object's display arguments (GenericArgs) count: instantiations are compared
// through them.
func mayMentionTypeParamNames(t types.Type, names map[string]types.Type, visited map[types.Type]bool, shadowed map[string]map[types.Type]bool) bool {
	if t == nil {
		return false
	}
	switch tt := t.(type) {
	case *types.Primitive, *types.LiteralType:
		return false
	case *types.TypeParameterType:
		if tt.Parameter == nil {
			return true
		}
		_, ok := names[tt.Parameter.Name]
		return ok
	case *types.ArrayType:
		return mayMentionTypeParamNames(tt.ElementType, names, visited, shadowed)
	case *types.ReadonlyType:
		return mayMentionTypeParamNames(tt.InnerType, names, visited, shadowed)
	case *types.UnionType:
		for _, m := range tt.Types {
			if mayMentionTypeParamNames(m, names, visited, shadowed) {
				return true
			}
		}
		return false
	case *types.IntersectionType:
		for _, m := range tt.Types {
			if mayMentionTypeParamNames(m, names, visited, shadowed) {
				return true
			}
		}
		return false
	case *types.InstantiatedType:
		// Substitution changes `Promise<Page>` or a forward `BrowserType<X>`
		// only through its arguments. Counting them as mentions made every
		// object reaching one (all of playwright's) a copy per substitution.
		for _, a := range tt.TypeArguments {
			if mayMentionTypeParamNames(a, names, visited, shadowed) {
				return true
			}
		}
		return false
	case *types.GenericTypeAliasForwardReference:
		for _, a := range tt.TypeArguments {
			if a == nil || mayMentionTypeParamNames(a, names, visited, shadowed) {
				return true
			}
		}
		return false
	case *types.InferType:
		// Declared by its conditional; substitution leaves it alone.
		return false
	case *types.ConditionalType:
		// Like tsc's couldContainTypeVariables. The true branch means the
		// conditional's own `infer T`, not an outer T: playwright's
		// Unboxed<Arg> (reachable from nearly every object) counted as
		// mentioning any T, so instantiating anything copied the graph.
		if mayMentionTypeParamNames(tt.CheckType, names, visited, shadowed) ||
			mayMentionTypeParamNames(tt.ExtendsType, names, visited, shadowed) ||
			mayMentionTypeParamNames(tt.FalseType, names, visited, shadowed) {
			return true
		}
		var inferred []*types.TypeParameter
		var inferNames []string
		collectInferTypeNames(tt.ExtendsType, &inferNames, map[types.Type]bool{})
		for _, n := range inferNames {
			inferred = append(inferred, &types.TypeParameter{Name: n})
		}
		trueNames, trueVisited := names, visited
		if unshadowed, shadows := unshadowedNames(names, inferred); shadows {
			trueNames, trueVisited = unshadowed, shadowedVisited(shadowed, unshadowed)
		}
		return mayMentionTypeParamNames(tt.TrueType, trueNames, trueVisited, shadowed)
	case *types.MappedType:
		// `[key in K]: V` binds its own key in V: hookable's nested mapped
		// types counted as mentioning every outer `key`, so expanding one
		// copied tsdown's whole hook graph per key.
		if mayMentionTypeParamNames(tt.ConstraintType, names, visited, shadowed) {
			return true
		}
		valueNames, valueVisited := names, visited
		if unshadowed, shadows := unshadowedNames(names, []*types.TypeParameter{{Name: tt.TypeParameter}}); shadows {
			valueNames, valueVisited = unshadowed, shadowedVisited(shadowed, unshadowed)
		}
		return mayMentionTypeParamNames(tt.ValueType, valueNames, valueVisited, shadowed)
	case *types.KeyofType:
		return mayMentionTypeParamNames(tt.OperandType, names, visited, shadowed)
	case *types.IndexedAccessType:
		return mayMentionTypeParamNames(tt.ObjectType, names, visited, shadowed) ||
			mayMentionTypeParamNames(tt.IndexType, names, visited, shadowed)
	case *types.TupleType:
		for _, e := range tt.ElementTypes {
			if mayMentionTypeParamNames(e, names, visited, shadowed) {
				return true
			}
		}
		return mayMentionTypeParamNames(tt.RestElementType, names, visited, shadowed)
	case *types.TemplateLiteralType:
		for _, part := range tt.Parts {
			if !part.IsLiteral && mayMentionTypeParamNames(part.Type, names, visited, shadowed) {
				return true
			}
		}
		return false
	case *types.ObjectType:
		if visited[t] {
			return false
		}
		visited[t] = true
		if tt.ClassMeta != nil && tt.ClassMeta.IsClassInstance && !tt.Pending {
			// A class instance mentions outer parameters only through its
			// type arguments (its members name the class's own parameters,
			// already replaced by those arguments) and the polymorphic this:
			// walking zod's class graph on every substitution never finished.
			if _, ok := names["this"]; ok {
				return true
			}
			for _, a := range tt.GenericArgs {
				if mayMentionTypeParamNames(a, names, visited, shadowed) {
					return true
				}
			}
			return false
		}
		for _, p := range tt.Props() {
			if mayMentionTypeParamNames(p, names, visited, shadowed) {
				return true
			}
		}
		for _, a := range tt.GenericArgs {
			if mayMentionTypeParamNames(a, names, visited, shadowed) {
				return true
			}
		}
		for _, b := range tt.Bases() {
			if mayMentionTypeParamNames(b, names, visited, shadowed) {
				return true
			}
		}
		for _, sigs := range [][]*types.Signature{tt.CallSigs(), tt.ConstructSigs()} {
			for _, sig := range sigs {
				if sig == nil {
					continue
				}
				sigNames, sigVisited := names, visited
				if unshadowed, shadows := unshadowedNames(names, sig.TypeParameters); shadows {
					// `waitForEvent<T>(...)` means its own T: an outer T
					// doesn't reach it.
					sigNames, sigVisited = unshadowed, shadowedVisited(shadowed, unshadowed)
				}
				for _, p := range sig.ParameterTypes {
					if mayMentionTypeParamNames(p, sigNames, sigVisited, shadowed) {
						return true
					}
				}
				if mayMentionTypeParamNames(sig.ReturnType, sigNames, sigVisited, shadowed) ||
					mayMentionTypeParamNames(sig.RestParameterType, sigNames, sigVisited, shadowed) ||
					mayMentionTypeParamNames(sig.ThisType, sigNames, sigVisited, shadowed) {
					return true
				}
				// Not the signature's own type parameters' constraints and
				// defaults: substitution keeps them, so counting them made
				// every object reaching an instantiated ElementHandle<T>
				// (`evaluate<R, Arg, O extends T = T>`) mention T forever.
			}
		}
		for _, sig := range tt.IndexSigs() {
			if sig != nil && (mayMentionTypeParamNames(sig.KeyType, names, visited, shadowed) || mayMentionTypeParamNames(sig.ValueType, names, visited, shadowed) || mayMentionTypeParamNames(sig.ConstraintType, names, visited, shadowed)) {
				return true
			}
		}
		return false
	case *types.ParameterizedForwardReferenceType:
		for _, a := range tt.TypeArguments {
			if mayMentionTypeParamNames(a, names, visited, shadowed) {
				return true
			}
		}
		return false
	}
	// substituteTypesWithVisited returns every other kind as it is (a
	// generic's declaration, typeof, an enum, a plain forward reference).
	return false
}

// shadowedVisited is the visited set of a walk for the names still in scope
// under a shadowing declaration. What is settled for fewer names isn't for
// all of them, so each set of names gets its own, shared by every place it
// is reached from: a fresh one per generic method or conditional re-walked
// playwright's graph from each of them.
func shadowedVisited(shadowed map[string]map[types.Type]bool, names map[string]types.Type) map[types.Type]bool {
	keys := make([]string, 0, len(names))
	for name := range names {
		keys = append(keys, name)
	}
	sort.Strings(keys)
	key := strings.Join(keys, ",")
	set := shadowed[key]
	if set == nil {
		set = make(map[types.Type]bool)
		shadowed[key] = set
	}
	return set
}

// unshadowedNames drops from names the type parameters a generic signature
// declares itself, reporting whether it declared any of them.
func unshadowedNames[V any](names map[string]V, own []*types.TypeParameter) (map[string]V, bool) {
	var out map[string]V
	for _, tp := range own {
		if tp == nil {
			continue
		}
		if _, ok := names[tp.Name]; !ok {
			continue
		}
		if out == nil {
			out = maps.Clone(names)
		}
		delete(out, tp.Name)
	}
	return out, out != nil
}

// substituteMembers fills in result's members: typ's, substituted.
func (c *Checker) substituteMembers(typ, result *types.ObjectType, substitution map[string]types.Type, visited map[types.Type]types.Type) {
	sub := func(t types.Type) types.Type { return c.substituteTypesWithVisited(t, substitution, visited) }
	result.Properties = make(map[string]types.Type, len(typ.Props()))
	result.OptionalProperties = make(map[string]bool, len(typ.OptionalProps()))
	result.ReadOnlyProperties = make(map[string]bool, len(typ.ReadOnlyProps()))
	// Inherited members come from the bases: ZodObject<..., Output>
	// reads _output through `extends ZodType<Output, ...>`.
	if bases := typ.Bases(); len(bases) > 0 {
		result.BaseTypes = make([]types.Type, len(bases))
		for i, base := range bases {
			result.BaseTypes[i] = sub(base)
		}
	}
	for _, propName := range typ.PropertyNames() {
		result.SetProperty(propName, sub(typ.Props()[propName]))
	}
	for propName, isOptional := range typ.OptionalProps() {
		result.OptionalProperties[propName] = isOptional
	}
	for propName, isReadOnly := range typ.ReadOnlyProps() {
		result.ReadOnlyProperties[propName] = isReadOnly
	}
	if sigs := typ.IndexSigs(); sigs != nil {
		index := make([]*types.IndexSignature, len(sigs))
		for i, sig := range sigs {
			index[i] = &types.IndexSignature{
				KeyType:        sub(sig.KeyType),
				ValueType:      sub(sig.ValueType),
				IsMapped:       sig.IsMapped,
				TypeParameter:  sig.TypeParameter,
				ConstraintType: sub(sig.ConstraintType),
			}
		}
		result.IndexSignatures = index
	}
	// Signature slices are assigned once complete: a recursive visit of
	// result meanwhile (it is in visited) must not see empty slots.
	if sigs := typ.CallSigs(); sigs != nil {
		result.CallSignatures = c.substituteSignatures(sigs, substitution, visited)
	}
	if sigs := typ.ConstructSigs(); sigs != nil {
		result.ConstructSignatures = c.substituteSignatures(sigs, substitution, visited)
	}
}

func (c *Checker) substituteSignatures(sigs []*types.Signature, substitution map[string]types.Type, visited map[types.Type]types.Type) []*types.Signature {
	out := make([]*types.Signature, len(sigs))
	for i, sig := range sigs {
		sigSubstitution, sigVisited := substitution, visited
		if unshadowed, shadows := unshadowedNames(substitution, sig.TypeParameters); shadows {
			if len(unshadowed) == 0 {
				out[i] = sig
				continue
			}
			// The memo belongs to the full substitution.
			sigSubstitution, sigVisited = unshadowed, make(map[types.Type]types.Type)
		}
		sub := func(t types.Type) types.Type { return c.substituteTypesWithVisited(t, sigSubstitution, sigVisited) }
		params := make([]types.Type, len(sig.ParameterTypes))
		for j, p := range sig.ParameterTypes {
			params[j] = sub(p)
		}
		out[i] = &types.Signature{
			TypeParameters:    sig.TypeParameters,
			ParameterNames:    sig.ParameterNames,
			ParameterTypes:    params,
			ReturnType:        sub(sig.ReturnType),
			OptionalParams:    sig.OptionalParams,
			IsVariadic:        sig.IsVariadic,
			StrictVariance:    sig.StrictVariance,
			RestParameterType: sub(sig.RestParameterType),
		}
	}
	return out
}
