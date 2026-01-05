package builtins

import (
	"strings"

	"paserati/pkg/types"
	"paserati/pkg/vm"
)

// Priority for Paserati (after standard globals)
const PriorityPaserati = 200

type PaseratiInitializer struct{}

func (p *PaseratiInitializer) Name() string {
	return "Paserati"
}

func (p *PaseratiInitializer) Priority() int {
	return PriorityPaserati
}

func (p *PaseratiInitializer) InitTypes(ctx *TypeContext) error {
	// Create method types
	toJSONSchemaType := types.NewFunctionType(&types.Signature{
		ParameterTypes: []types.Type{},
		ReturnType:     types.Any,
	})
	toStringType := types.NewFunctionType(&types.Signature{
		ParameterTypes: []types.Type{},
		ReturnType:     types.String,
	})

	// Create Type interface - represents a runtime type descriptor
	typeInterface := types.NewObjectType().
		WithProperty("kind", types.String).
		WithOptionalProperty("name", types.String).
		WithOptionalProperty("properties", types.Any).     // For object types
		WithOptionalProperty("elementType", types.Any).    // For array types
		WithOptionalProperty("types", types.Any).          // For union types
		WithOptionalProperty("parameters", types.Any).     // For function types
		WithOptionalProperty("returnType", types.Any).     // For function types
		WithProperty("toJSONSchema", toJSONSchemaType).    // Method to convert to JSON Schema
		WithProperty("toString", toStringType)             // Method to convert to TypeScript-like string

	// Define Type interface for users
	if err := ctx.DefineTypeAlias("Type", typeInterface); err != nil {
		return err
	}

	// Create the Paserati namespace type
	// reflect<T>() is a compile-time intrinsic that returns a Type descriptor
	// We use a marker type that the checker will recognize
	reflectMethodType := types.NewObjectType().WithCallSignature(&types.Signature{
		ParameterTypes: []types.Type{},
		ReturnType:     typeInterface,
	})
	// Mark this as an intrinsic
	reflectMethodType.IsReflectIntrinsic = true

	paseratiType := types.NewObjectType().
		WithProperty("reflect", reflectMethodType)

	// Define Paserati namespace in global environment
	return ctx.DefineGlobal("Paserati", paseratiType)
}

func (p *PaseratiInitializer) InitRuntime(ctx *RuntimeContext) error {
	vmInstance := ctx.VM

	// Create TypeDescriptor prototype with methods
	typeProto := vm.NewObject(vmInstance.ObjectPrototype).AsPlainObject()
	typeProto.SetOwnNonEnumerable("toJSONSchema", vm.NewNativeFunction(0, false, "toJSONSchema", func(args []vm.Value) (vm.Value, error) {
		// Get 'this' from the VM (the type descriptor object)
		thisVal := vmInstance.GetThis()
		return typeDescriptorToJSONSchema(vmInstance, thisVal)
	}))
	typeProto.SetOwnNonEnumerable("toString", vm.NewNativeFunction(0, false, "toString", func(args []vm.Value) (vm.Value, error) {
		// Get 'this' from the VM (the type descriptor object)
		thisVal := vmInstance.GetThis()
		return typeDescriptorToString(thisVal), nil
	}))

	// Create Paserati object
	paseratiObj := vm.NewObject(vmInstance.ObjectPrototype).AsPlainObject()

	// The reflect method is a placeholder - actual work is done by the compiler
	// If somehow called at runtime, it returns an error
	paseratiObj.SetOwnNonEnumerable("reflect", vm.NewNativeFunction(0, false, "reflect", func(args []vm.Value) (vm.Value, error) {
		// This should never be called - the compiler replaces it
		return vm.Undefined, nil
	}))

	// Store TypePrototype on Paserati so compiler can access it
	paseratiObj.SetOwnNonEnumerable("TypePrototype", vm.NewValueFromPlainObject(typeProto))

	// Register Paserati object as global
	return ctx.DefineGlobal("Paserati", vm.NewValueFromPlainObject(paseratiObj))
}

// schemaContext tracks state during JSON Schema generation
type schemaContext struct {
	vmInstance *vm.VM
	defs       map[string]vm.Value // Named type definitions
	isRoot     bool                // Whether this is the root call
}

// typeDescriptorToJSONSchema converts a Type descriptor object to JSON Schema format
func typeDescriptorToJSONSchema(vmInstance *vm.VM, typeDesc vm.Value) (vm.Value, error) {
	ctx := &schemaContext{
		vmInstance: vmInstance,
		defs:       make(map[string]vm.Value),
		isRoot:     true,
	}
	return ctx.convert(typeDesc)
}

// convert performs the actual conversion with context tracking
func (ctx *schemaContext) convert(typeDesc vm.Value) (vm.Value, error) {
	if typeDesc.IsUndefined() || typeDesc.Type() == vm.TypeNull {
		return vm.Undefined, nil
	}

	obj := typeDesc.AsPlainObject()
	if obj == nil {
		return vm.Undefined, nil
	}

	// Get the "kind" property
	kindVal, exists := obj.GetOwn("kind")
	if !exists {
		return vm.Undefined, nil
	}
	kind := vm.AsString(kindVal)

	// Create result schema object
	schema := vm.NewObject(ctx.vmInstance.ObjectPrototype).AsPlainObject()

	// Add $schema only at root level
	wasRoot := ctx.isRoot
	if ctx.isRoot {
		schema.SetOwn("$schema", vm.NewString("https://json-schema.org/draft/2020-12/schema"))
		ctx.isRoot = false
	}

	switch kind {
	case "primitive":
		// {kind: "primitive", name: "string"} -> {type: "string"}
		nameVal, _ := obj.GetOwn("name")
		name := vm.AsString(nameVal)

		switch name {
		case "string":
			schema.SetOwn("type", vm.NewString("string"))
		case "number":
			schema.SetOwn("type", vm.NewString("number"))
		case "boolean":
			schema.SetOwn("type", vm.NewString("boolean"))
		case "null":
			schema.SetOwn("type", vm.NewString("null"))
		case "undefined":
			// JSON Schema doesn't have undefined, use null
			schema.SetOwn("type", vm.NewString("null"))
		case "any":
			// Any type - no restrictions
			// Empty schema {} accepts anything
		case "unknown":
			// Unknown type - no restrictions
		default:
			schema.SetOwn("type", vm.NewString(name))
		}

	case "literal":
		// {kind: "literal", value: "foo", baseType: "string"} -> {const: "foo"}
		valueVal, _ := obj.GetOwn("value")
		schema.SetOwn("const", valueVal)

	case "array":
		// {kind: "array", elementType: T} -> {type: "array", items: toJSONSchema(T)}
		schema.SetOwn("type", vm.NewString("array"))
		elemTypeVal, exists := obj.GetOwn("elementType")
		if exists {
			itemsSchema, err := ctx.convert(elemTypeVal)
			if err != nil {
				return vm.Undefined, err
			}
			schema.SetOwn("items", itemsSchema)
		}

	case "tuple":
		// {kind: "tuple", elementTypes: [T1, T2]} -> {type: "array", prefixItems: [...], items: false}
		schema.SetOwn("type", vm.NewString("array"))
		elemTypesVal, exists := obj.GetOwn("elementTypes")
		if exists && !elemTypesVal.IsUndefined() && elemTypesVal.IsArray() {
			arr := elemTypesVal.AsArray()
			prefixItemsVal := vm.NewArrayWithLength(0)
			prefixItemsArr := prefixItemsVal.AsArray()
			for i := 0; i < arr.Length(); i++ {
				elemType := arr.Get(i)
				itemSchema, err := ctx.convert(elemType)
				if err != nil {
					return vm.Undefined, err
				}
				prefixItemsArr.Append(itemSchema)
			}
			schema.SetOwn("prefixItems", prefixItemsVal)
			schema.SetOwn("items", vm.False)
		}

	case "object":
		// {kind: "object", properties: {...}, baseTypes: [...]} -> {type: "object", properties: {...}, required: [...]}
		schema.SetOwn("type", vm.NewString("object"))

		schemaProps := vm.NewObject(ctx.vmInstance.ObjectPrototype).AsPlainObject()
		requiredVal := vm.NewArrayWithLength(0)
		requiredArr := requiredVal.AsArray()
		hasProperties := false

		// First, collect properties from base types (inherited properties)
		baseTypesVal, hasBaseTypes := obj.GetOwn("baseTypes")
		if hasBaseTypes && !baseTypesVal.IsUndefined() && baseTypesVal.IsArray() {
			baseTypesArr := baseTypesVal.AsArray()
			for i := 0; i < baseTypesArr.Length(); i++ {
				baseType := baseTypesArr.Get(i)
				ctx.collectPropertiesFromType(baseType, schemaProps, requiredArr, &hasProperties)
			}
		}

		// Then add own properties (may override inherited ones)
		propsVal, exists := obj.GetOwn("properties")
		if exists && !propsVal.IsUndefined() {
			propsObj := propsVal.AsPlainObject()
			if propsObj != nil {
				hasProperties = true
				ctx.addPropertiesToSchema(propsObj, schemaProps, requiredArr)
			}
		}

		if hasProperties {
			schema.SetOwn("properties", vm.NewValueFromPlainObject(schemaProps))
			if requiredArr.Length() > 0 {
				schema.SetOwn("required", requiredVal)
			}
		}

		// Handle index signatures -> additionalProperties
		indexSigsVal, exists := obj.GetOwn("indexSignatures")
		if exists && !indexSigsVal.IsUndefined() && indexSigsVal.IsArray() {
			arr := indexSigsVal.AsArray()
			if arr.Length() > 0 {
				// Use the first index signature's value type for additionalProperties
				firstSig := arr.Get(0)
				if sigObj := firstSig.AsPlainObject(); sigObj != nil {
					valueTypeVal, _ := sigObj.GetOwn("valueType")
					additionalSchema, err := ctx.convert(valueTypeVal)
					if err != nil {
						return vm.Undefined, err
					}
					schema.SetOwn("additionalProperties", additionalSchema)
				}
			}
		}

	case "union":
		// Check if this is a union of string literals -> use enum
		typesVal, exists := obj.GetOwn("types")
		if exists && !typesVal.IsUndefined() && typesVal.IsArray() {
			arr := typesVal.AsArray()

			// Check if all members are string or number literals
			allStringLiterals := true
			allNumberLiterals := true
			allLiterals := true

			for i := 0; i < arr.Length(); i++ {
				memberType := arr.Get(i)
				memberObj := memberType.AsPlainObject()
				if memberObj == nil {
					allLiterals = false
					allStringLiterals = false
					allNumberLiterals = false
					break
				}
				memberKind, _ := memberObj.GetOwn("kind")
				if vm.AsString(memberKind) != "literal" {
					allLiterals = false
					allStringLiterals = false
					allNumberLiterals = false
					break
				}
				baseType, _ := memberObj.GetOwn("baseType")
				baseTypeStr := vm.AsString(baseType)
				if baseTypeStr != "string" {
					allStringLiterals = false
				}
				if baseTypeStr != "number" {
					allNumberLiterals = false
				}
			}

			// If all are string literals or all are number literals, use enum
			if allLiterals && (allStringLiterals || allNumberLiterals) {
				enumVal := vm.NewArrayWithLength(0)
				enumArr := enumVal.AsArray()
				for i := 0; i < arr.Length(); i++ {
					memberType := arr.Get(i)
					memberObj := memberType.AsPlainObject()
					valueVal, _ := memberObj.GetOwn("value")
					enumArr.Append(valueVal)
				}
				if allStringLiterals {
					schema.SetOwn("type", vm.NewString("string"))
				} else {
					schema.SetOwn("type", vm.NewString("number"))
				}
				schema.SetOwn("enum", enumVal)
			} else {
				// General union -> anyOf
				anyOfVal := vm.NewArrayWithLength(0)
				anyOfArr := anyOfVal.AsArray()
				for i := 0; i < arr.Length(); i++ {
					memberType := arr.Get(i)
					memberSchema, err := ctx.convert(memberType)
					if err != nil {
						return vm.Undefined, err
					}
					anyOfArr.Append(memberSchema)
				}
				schema.SetOwn("anyOf", anyOfVal)
			}
		}

	case "intersection":
		// {kind: "intersection", types: [...]} -> {allOf: [...]}
		typesVal, exists := obj.GetOwn("types")
		if exists && !typesVal.IsUndefined() && typesVal.IsArray() {
			arr := typesVal.AsArray()
			allOfVal := vm.NewArrayWithLength(0)
			allOfArr := allOfVal.AsArray()
			for i := 0; i < arr.Length(); i++ {
				memberType := arr.Get(i)
				memberSchema, err := ctx.convert(memberType)
				if err != nil {
					return vm.Undefined, err
				}
				allOfArr.Append(memberSchema)
			}
			schema.SetOwn("allOf", allOfVal)
		}

	case "class":
		// For class types, convert the instance type
		instanceTypeVal, exists := obj.GetOwn("instanceType")
		if exists && !instanceTypeVal.IsUndefined() {
			return ctx.convert(instanceTypeVal)
		}

	default:
		// Unknown kind - return empty schema
	}

	// Add $defs at root level if we have any
	if wasRoot && len(ctx.defs) > 0 {
		defsObj := vm.NewObject(ctx.vmInstance.ObjectPrototype).AsPlainObject()
		for name, defSchema := range ctx.defs {
			defsObj.SetOwn(name, defSchema)
		}
		schema.SetOwn("$defs", vm.NewValueFromPlainObject(defsObj))
	}

	return vm.NewValueFromPlainObject(schema), nil
}

// collectPropertiesFromType recursively collects properties from a type descriptor (for inheritance)
func (ctx *schemaContext) collectPropertiesFromType(typeDesc vm.Value, schemaProps *vm.PlainObject, requiredArr *vm.ArrayObject, hasProperties *bool) {
	if typeDesc.IsUndefined() || typeDesc.Type() == vm.TypeNull {
		return
	}

	obj := typeDesc.AsPlainObject()
	if obj == nil {
		return
	}

	kindVal, exists := obj.GetOwn("kind")
	if !exists {
		return
	}
	kind := vm.AsString(kindVal)

	if kind != "object" {
		return
	}

	// First recursively collect from this type's base types
	baseTypesVal, hasBaseTypes := obj.GetOwn("baseTypes")
	if hasBaseTypes && !baseTypesVal.IsUndefined() && baseTypesVal.IsArray() {
		baseTypesArr := baseTypesVal.AsArray()
		for i := 0; i < baseTypesArr.Length(); i++ {
			baseType := baseTypesArr.Get(i)
			ctx.collectPropertiesFromType(baseType, schemaProps, requiredArr, hasProperties)
		}
	}

	// Then collect own properties
	propsVal, propsExist := obj.GetOwn("properties")
	if propsExist && !propsVal.IsUndefined() {
		propsObj := propsVal.AsPlainObject()
		if propsObj != nil {
			*hasProperties = true
			ctx.addPropertiesToSchema(propsObj, schemaProps, requiredArr)
		}
	}
}

// addPropertiesToSchema adds properties from a type descriptor to the schema
func (ctx *schemaContext) addPropertiesToSchema(propsObj *vm.PlainObject, schemaProps *vm.PlainObject, requiredArr *vm.ArrayObject) {
	keys := propsObj.OwnKeys()
	for _, key := range keys {
		propDescVal, _ := propsObj.GetOwn(key)
		propDescObj := propDescVal.AsPlainObject()
		if propDescObj == nil {
			continue
		}

		// Get the type descriptor for this property
		propTypeVal, _ := propDescObj.GetOwn("type")
		propTypeObj := propTypeVal.AsPlainObject()

		// Skip methods (properties with callSignatures) - they can't be serialized
		if propTypeObj != nil {
			callSigsVal, hasCallSigs := propTypeObj.GetOwn("callSignatures")
			if hasCallSigs && !callSigsVal.IsUndefined() && callSigsVal.IsArray() {
				if callSigsVal.AsArray().Length() > 0 {
					continue // Skip this property - it's a method
				}
			}
		}

		propSchema, err := ctx.convert(propTypeVal)
		if err != nil {
			continue // Skip properties that fail to convert
		}
		schemaProps.SetOwn(key, propSchema)

		// Check if property is optional - but don't add duplicates to required
		optionalVal, exists := propDescObj.GetOwn("optional")
		isOptional := exists && optionalVal.IsTruthy()
		if !isOptional {
			// Check if already in required array
			alreadyRequired := false
			for i := 0; i < requiredArr.Length(); i++ {
				if vm.AsString(requiredArr.Get(i)) == key {
					alreadyRequired = true
					break
				}
			}
			if !alreadyRequired {
				requiredArr.Append(vm.NewString(key))
			}
		}
	}
}

// typeDescriptorToString converts a Type descriptor object to a TypeScript-like string
func typeDescriptorToString(typeDesc vm.Value) vm.Value {
	result := typeToString(typeDesc)
	return vm.NewString(result)
}

// typeToString recursively converts a type descriptor to string
func typeToString(typeDesc vm.Value) string {
	if typeDesc.IsUndefined() || typeDesc.Type() == vm.TypeNull {
		return "unknown"
	}

	obj := typeDesc.AsPlainObject()
	if obj == nil {
		return "unknown"
	}

	kindVal, exists := obj.GetOwn("kind")
	if !exists {
		return "unknown"
	}
	kind := vm.AsString(kindVal)

	switch kind {
	case "primitive":
		nameVal, _ := obj.GetOwn("name")
		return vm.AsString(nameVal)

	case "literal":
		valueVal, _ := obj.GetOwn("value")
		baseTypeVal, _ := obj.GetOwn("baseType")
		baseType := vm.AsString(baseTypeVal)
		if baseType == "string" {
			return "\"" + vm.AsString(valueVal) + "\""
		}
		return valueVal.Inspect()

	case "array":
		elemTypeVal, exists := obj.GetOwn("elementType")
		if exists {
			elemStr := typeToString(elemTypeVal)
			// Use Array<T> for complex types, T[] for simple
			if needsParens(elemStr) {
				return "Array<" + elemStr + ">"
			}
			return elemStr + "[]"
		}
		return "unknown[]"

	case "tuple":
		elemTypesVal, exists := obj.GetOwn("elementTypes")
		if exists && elemTypesVal.IsArray() {
			arr := elemTypesVal.AsArray()
			parts := make([]string, arr.Length())
			for i := 0; i < arr.Length(); i++ {
				parts[i] = typeToString(arr.Get(i))
			}
			return "[" + joinStrings(parts, ", ") + "]"
		}
		return "[]"

	case "object":
		// Check if it has a name (named interface/type)
		nameVal, hasName := obj.GetOwn("name")
		if hasName && !nameVal.IsUndefined() {
			name := vm.AsString(nameVal)
			if name != "" {
				return name
			}
		}

		// Check for call signatures (function types represented as objects)
		callSigsVal, hasCallSigs := obj.GetOwn("callSignatures")
		if hasCallSigs && callSigsVal.IsArray() {
			arr := callSigsVal.AsArray()
			if arr.Length() > 0 {
				// Get the first call signature
				sigVal := arr.Get(0)
				sigObj := sigVal.AsPlainObject()
				if sigObj != nil {
					paramsVal, _ := sigObj.GetOwn("parameters")
					returnVal, _ := sigObj.GetOwn("returnType")

					paramStr := "()"
					if paramsVal.IsArray() {
						paramsArr := paramsVal.AsArray()
						parts := make([]string, paramsArr.Length())
						for i := 0; i < paramsArr.Length(); i++ {
							paramDescVal := paramsArr.Get(i)
							paramDescObj := paramDescVal.AsPlainObject()
							if paramDescObj != nil {
								paramTypeVal, _ := paramDescObj.GetOwn("type")
								parts[i] = typeToString(paramTypeVal)
							} else {
								parts[i] = "unknown"
							}
						}
						paramStr = "(" + joinStrings(parts, ", ") + ")"
					}

					returnStr := "void"
					if !returnVal.IsUndefined() {
						returnStr = typeToString(returnVal)
					}

					return paramStr + " => " + returnStr
				}
			}
		}

		// Anonymous object type - show structure
		propsVal, exists := obj.GetOwn("properties")
		if !exists || propsVal.IsUndefined() {
			return "{}"
		}
		propsObj := propsVal.AsPlainObject()
		if propsObj == nil {
			return "{}"
		}

		keys := propsObj.OwnKeys()
		if len(keys) == 0 {
			return "{}"
		}

		parts := make([]string, 0, len(keys))
		for _, key := range keys {
			propDescVal, _ := propsObj.GetOwn(key)
			propDescObj := propDescVal.AsPlainObject()
			if propDescObj == nil {
				continue
			}
			propTypeVal, _ := propDescObj.GetOwn("type")
			optionalVal, hasOptional := propDescObj.GetOwn("optional")
			isOptional := hasOptional && optionalVal.IsTruthy()

			propStr := key
			if isOptional {
				propStr += "?"
			}
			propStr += ": " + typeToString(propTypeVal)
			parts = append(parts, propStr)
		}
		return "{ " + joinStrings(parts, "; ") + " }"

	case "union":
		typesVal, exists := obj.GetOwn("types")
		if exists && typesVal.IsArray() {
			arr := typesVal.AsArray()
			parts := make([]string, arr.Length())
			for i := 0; i < arr.Length(); i++ {
				parts[i] = typeToString(arr.Get(i))
			}
			return joinStrings(parts, " | ")
		}
		return "unknown"

	case "intersection":
		typesVal, exists := obj.GetOwn("types")
		if exists && typesVal.IsArray() {
			arr := typesVal.AsArray()
			parts := make([]string, arr.Length())
			for i := 0; i < arr.Length(); i++ {
				parts[i] = typeToString(arr.Get(i))
			}
			return joinStrings(parts, " & ")
		}
		return "unknown"

	case "function":
		paramsVal, _ := obj.GetOwn("parameters")
		returnVal, _ := obj.GetOwn("returnType")

		paramStr := "()"
		if paramsVal.IsArray() {
			arr := paramsVal.AsArray()
			parts := make([]string, arr.Length())
			for i := 0; i < arr.Length(); i++ {
				parts[i] = typeToString(arr.Get(i))
			}
			paramStr = "(" + joinStrings(parts, ", ") + ")"
		}

		returnStr := "void"
		if !returnVal.IsUndefined() {
			returnStr = typeToString(returnVal)
		}

		return paramStr + " => " + returnStr

	case "class":
		nameVal, _ := obj.GetOwn("name")
		return vm.AsString(nameVal)

	default:
		return "unknown"
	}
}

// needsParens returns true if the type string needs parentheses when used in array notation
func needsParens(s string) bool {
	return strings.Contains(s, "|") || strings.Contains(s, "&") || strings.Contains(s, "=>")
}

// joinStrings joins strings with a separator
func joinStrings(parts []string, sep string) string {
	return strings.Join(parts, sep)
}
