package errors

// --- TypeScript diagnostic codes ---
//
// Diagnostics that correspond to a TypeScript compiler error carry that
// compiler's code and its exact wording, so `paserati-testtsc -strict-errors`
// can verify we report the *same* diagnostic TypeScript would, not merely that
// we reported something. Diagnostics with no TypeScript equivalent — and those
// not yet mapped — keep their PS code and are reported as unmapped.
const (
	TS1003  = "TS1003"  // Identifier expected
	TS1005  = "TS1005"  // '<token>' expected
	TS1015  = "TS1015"  // Parameter cannot have question mark and initializer
	TS1016  = "TS1016"  // A required parameter cannot follow an optional parameter
	TS1042  = "TS1042"  // 'X' modifier cannot be used here
	TS1051  = "TS1051"  // A 'set' accessor cannot have an optional parameter
	TS1052  = "TS1052"  // A 'set' accessor parameter cannot have an initializer
	TS1061  = "TS1061"  // Enum member must have initializer
	TS1092  = "TS1092"  // Type parameters cannot appear on a constructor declaration
	TS1093  = "TS1093"  // Type annotation cannot appear on a constructor declaration
	TS1100  = "TS1100"  // Invalid use of 'X' in strict mode
	TS1102  = "TS1102"  // 'delete' cannot be called on an identifier in strict mode
	TS1104  = "TS1104"  // A 'continue' statement can only be used within an enclosing iteration statement
	TS1105  = "TS1105"  // A 'break' statement can only be used within an enclosing iteration or switch statement
	TS1107  = "TS1107"  // Jump target cannot cross function boundary
	TS1108  = "TS1108"  // A 'return' statement can only be used within a function body
	TS1109  = "TS1109"  // Expression expected
	TS1114  = "TS1114"  // Duplicate label 'X'
	TS1115  = "TS1115"  // A 'continue' statement can only jump to a label of an enclosing iteration statement
	TS1116  = "TS1116"  // A 'break' statement can only jump to a label of an enclosing statement
	TS1117  = "TS1117"  // An object literal cannot have multiple properties with the same name
	TS1127  = "TS1127"  // Invalid character
	TS1155  = "TS1155"  // 'const' declarations must be initialized
	TS1184  = "TS1184"  // Modifiers cannot appear here
	TS1196  = "TS1196"  // Catch clause variable type annotation must be 'any' or 'unknown' if specified
	TS1206  = "TS1206"  // Decorators are not valid here
	TS1212  = "TS1212"  // Identifier expected. 'X' is a reserved word in strict mode
	TS1213  = "TS1213"  // ... Class definitions are automatically in strict mode
	TS1214  = "TS1214"  // ... Modules are automatically in strict mode
	TS1263  = "TS1263"  // Declarations with initializers cannot also have definite assignment assertions
	TS1346  = "TS1346"  // This parameter is not allowed with 'use strict' directive
	TS1347  = "TS1347"  // 'use strict' directive cannot be used with non-simple parameter list
	TS1360  = "TS1360"  // Type 'X' does not satisfy the expected type 'Y'
	TS1492  = "TS1492"  // 'using'/'await using' declarations may not have binding patterns
	TS2300  = "TS2300"  // Duplicate identifier 'X'
	TS2304  = "TS2304"  // Cannot find name 'X'
	TS2312  = "TS2312"  // An interface can only extend an object type or intersection of object types with statically known members
	TS2314  = "TS2314"  // Generic type 'X' requires N type argument(s)
	TS2315  = "TS2315"  // Type 'X' is not generic
	TS2322  = "TS2322"  // Type 'X' is not assignable to type 'Y'
	TS2335  = "TS2335"  // 'super' can only be referenced in a derived class
	TS2337  = "TS2337"  // Super calls are not permitted outside constructors or in nested functions inside constructors
	TS2339  = "TS2339"  // Property 'X' does not exist on type 'Y'
	TS2341  = "TS2341"  // Property 'X' is private and only accessible within class 'C'
	TS2344  = "TS2344"  // Type 'X' does not satisfy the constraint 'Y'
	TS2345  = "TS2345"  // Argument of type 'X' is not assignable to parameter of type 'Y'
	TS2347  = "TS2347"  // Untyped function calls may not accept type arguments
	TS2348  = "TS2348"  // Value of type 'X' is not callable. Did you mean to include 'new'?
	TS2349  = "TS2349"  // This expression is not callable
	TS2351  = "TS2351"  // This expression is not constructable
	TS2352  = "TS2352"  // Conversion of type 'X' to type 'Y' may be a mistake
	TS2358  = "TS2358"  // The left-hand side of an 'instanceof' expression must be of type 'any', an object type or a type parameter
	TS2359  = "TS2359"  // The right-hand side of an 'instanceof' expression must be either of type 'any', a class, function, or other type assignable to the 'Function' interface type, or an object type with a 'Symbol.hasInstance' method
	TS2362  = "TS2362"  // The left-hand side of an arithmetic operation must be of type 'any', 'number', 'bigint' or an enum type
	TS2363  = "TS2363"  // The right-hand side of an arithmetic operation must be of type 'any', 'number', 'bigint' or an enum type
	TS2365  = "TS2365"  // Operator 'X' cannot be applied to types 'A' and 'B'
	TS2367  = "TS2367"  // This comparison appears to be unintentional
	TS2369  = "TS2369"  // A parameter property is only allowed in a constructor implementation
	TS2370  = "TS2370"  // A rest parameter must be of an array type
	TS2371  = "TS2371"  // A parameter initializer is only allowed in a function or constructor implementation
	TS2374  = "TS2374"  // Duplicate index signature for type 'X'
	TS2378  = "TS2378"  // A 'get' accessor must return a value
	TS2390  = "TS2390"  // Constructor implementation is missing
	TS2391  = "TS2391"  // Function implementation is missing or not immediately following the declaration
	TS2393  = "TS2393"  // Duplicate function implementation
	TS2394  = "TS2394"  // This overload signature is not compatible with its implementation signature
	TS2411  = "TS2411"  // Property 'X' of type 'T' is not assignable to 'string' index type 'U'
	TS2427  = "TS2427"  // Interface name cannot be 'X'
	TS2428  = "TS2428"  // All declarations of 'X' must have identical type parameters
	TS2445  = "TS2445"  // Property 'X' is protected and only accessible within class 'C' and its subclasses
	TS2446  = "TS2446"  // Property 'X' is protected and only accessible through an instance of class 'C'
	TS2454  = "TS2454"  // Variable 'X' is used before being assigned
	TS2461  = "TS2461"  // Type 'X' is not an array type
	TS2464  = "TS2464"  // A computed property name must be of type 'string', 'number', 'symbol', or 'any'
	TS2469  = "TS2469"  // The 'X' operator cannot be applied to type 'symbol'
	TS2488  = "TS2488"  // Type 'X' must have a '[Symbol.iterator]()' method that returns an iterator
	TS2493  = "TS2493"  // Tuple type 'T' of length 'N' has no element at index 'M'
	TS2507  = "TS2507"  // Type 'X' is not a constructor function type
	TS2511  = "TS2511"  // Cannot create an instance of an abstract class
	TS2514  = "TS2514"  // A tuple type cannot be indexed with a negative value
	TS2515  = "TS2515"  // Non-abstract class 'C' does not implement inherited abstract member X from class 'B'
	TS2531  = "TS2531"  // Object is possibly 'null'
	TS2532  = "TS2532"  // Object is possibly 'undefined'
	TS2533  = "TS2533"  // Object is possibly 'null' or 'undefined'
	TS2538  = "TS2538"  // Type 'X' cannot be used as an index type
	TS2540  = "TS2540"  // Cannot assign to 'X' because it is a read-only property
	TS2552  = "TS2552"  // Cannot find name 'X'. Did you mean 'Y'?
	TS2554  = "TS2554"  // Expected N arguments, but got M
	TS2555  = "TS2555"  // Expected at least N arguments, but got M
	TS2556  = "TS2556"  // A spread argument must either have a tuple type or be passed to a rest parameter
	TS2558  = "TS2558"  // Expected N type arguments, but got M
	TS2564  = "TS2564"  // Property 'X' has no initializer and is not definitely assigned in the constructor
	TS2571  = "TS2571"  // Object is of type 'unknown'
	TS2575  = "TS2575"  // No overload expects N arguments, but overloads do exist that expect either A or B arguments
	TS2588  = "TS2588"  // Cannot assign to 'X' because it is a constant
	TS2630  = "TS2630"  // Cannot assign to 'X' because it is a function
	TS2654  = "TS2654"  // Non-abstract class 'C' is missing implementations for the following members of 'B': ...
	TS2655  = "TS2655"  // Non-abstract class 'C' is missing implementations ... and N more
	TS2689  = "TS2689"  // Cannot extend an interface 'X'. Did you mean 'implements'?
	TS2693  = "TS2693"  // 'X' only refers to a type, but is being used as a value here
	TS2694  = "TS2694"  // Namespace 'X' has no exported member 'Y'
	TS2695  = "TS2695"  // Left side of comma operator is unused and has no side effects
	TS2698  = "TS2698"  // Spread types may only be created from object types
	TS2703  = "TS2703"  // The operand of a 'delete' operator must be a property reference
	TS2707  = "TS2707"  // Generic type 'X' requires between N and M type arguments
	TS2721  = "TS2721"  // Cannot invoke an object which is possibly 'null'
	TS2722  = "TS2722"  // Cannot invoke an object which is possibly 'undefined'
	TS2723  = "TS2723"  // Cannot invoke an object which is possibly 'null' or 'undefined'
	TS2743  = "TS2743"  // No overload expects N type arguments, but overloads do exist that expect either A or B
	TS2769  = "TS2769"  // No overload matches this call
	TS2840  = "TS2840"  // An interface cannot extend a primitive type like 'X'. It can only extend other named object types.
	TS4112  = "TS4112"  // This member cannot have an 'override' modifier because its containing class 'X' does not extend another class
	TS4113  = "TS4113"  // This member cannot have an 'override' modifier because it is not declared in the base class 'X'
	TS4114  = "TS4114"  // This member must have an 'override' modifier because it overrides a member in the base class 'X'
	TS4117  = "TS4117"  // This member cannot have an 'override' modifier because it is not declared in the base class 'X'. Did you mean 'Y'?
	TS17005 = "TS17005" // A constructor cannot contain a 'super' call when its class extends 'null'
	TS18013 = "TS18013" // Property 'X' is not accessible outside class 'Y'
	TS18046 = "TS18046" // 'X' is of type 'unknown'
	TS18047 = "TS18047" // 'X' is possibly 'null'
	TS18048 = "TS18048" // 'X' is possibly 'undefined'
	TS18049 = "TS18049" // 'X' is possibly 'null' or 'undefined'
	TS18050 = "TS18050" // The value 'null'/'undefined' cannot be used here
)

// IsTSCode reports whether an error code is a TypeScript diagnostic code rather
// than a Paserati-internal PS code.
func IsTSCode(code string) bool {
	return len(code) > 2 && code[0] == 'T' && code[1] == 'S'
}

// TSCode returns the TypeScript diagnostic code an error maps to, or "" when
// the diagnostic has no TypeScript equivalent or has not been mapped yet.
func TSCode(e PaseratiError) string {
	if e == nil {
		return ""
	}
	if code := e.Code(); IsTSCode(code) {
		return code
	}
	return ""
}
