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
	TS1100  = "TS1100"  // Invalid use of 'X' in strict mode
	TS1109  = "TS1109"  // Expression expected
	TS1196  = "TS1196"  // Catch clause variable type annotation must be 'any' or 'unknown' if specified
	TS1042  = "TS1042"  // 'X' modifier cannot be used here
	TS1127  = "TS1127"  // Invalid character
	TS1206  = "TS1206"  // Decorators are not valid here
	TS1492  = "TS1492"  // 'using'/'await using' declarations may not have binding patterns
	TS1212  = "TS1212"  // Identifier expected. 'X' is a reserved word in strict mode
	TS1213  = "TS1213"  // ... Class definitions are automatically in strict mode
	TS1214  = "TS1214"  // ... Modules are automatically in strict mode
	TS1263  = "TS1263"  // Declarations with initializers cannot also have definite assignment assertions
	TS2300  = "TS2300"  // Duplicate identifier 'X'
	TS2304  = "TS2304"  // Cannot find name 'X'
	TS2322  = "TS2322"  // Type 'X' is not assignable to type 'Y'
	TS2339  = "TS2339"  // Property 'X' does not exist on type 'Y'
	TS2461  = "TS2461"  // Type 'X' is not an array type
	TS2345  = "TS2345"  // Argument of type 'X' is not assignable to parameter of type 'Y'
	TS2362  = "TS2362"  // The left-hand side of an arithmetic operation must be of type 'any', 'number', 'bigint' or an enum type
	TS2363  = "TS2363"  // The right-hand side of an arithmetic operation must be of type 'any', 'number', 'bigint' or an enum type
	TS2365  = "TS2365"  // Operator 'X' cannot be applied to types 'A' and 'B'
	TS2367  = "TS2367"  // This comparison appears to be unintentional
	TS2378  = "TS2378"  // A 'get' accessor must return a value
	TS2411  = "TS2411"  // Property 'X' of type 'T' is not assignable to 'string' index type 'U'
	TS2454  = "TS2454"  // Variable 'X' is used before being assigned
	TS2464  = "TS2464"  // A computed property name must be of type 'string', 'number', 'symbol', or 'any'
	TS2540  = "TS2540"  // Cannot assign to 'X' because it is a read-only property
	TS2552  = "TS2552"  // Cannot find name 'X'. Did you mean 'Y'?
	TS2554  = "TS2554"  // Expected N arguments, but got M
	TS2564  = "TS2564"  // Property 'X' has no initializer and is not definitely assigned in the constructor
	TS2695  = "TS2695"  // Left side of comma operator is unused and has no side effects
	TS18013 = "TS18013" // Property 'X' is not accessible outside class 'Y'
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
