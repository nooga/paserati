package errors

// --- TypeScript diagnostic codes ---
//
// Diagnostics that correspond to a TypeScript compiler error carry that
// compiler's code and its exact wording, so `paserati-testtsc -strict-errors`
// can verify we report the *same* diagnostic TypeScript would, not merely that
// we reported something. Diagnostics with no TypeScript equivalent — and those
// not yet mapped — keep their PS code and are reported as unmapped.
const (
	TS1005  = "TS1005"  // '<token>' expected
	TS2300  = "TS2300"  // Duplicate identifier 'X'
	TS2304  = "TS2304"  // Cannot find name 'X'
	TS2322  = "TS2322"  // Type 'X' is not assignable to type 'Y'
	TS2339  = "TS2339"  // Property 'X' does not exist on type 'Y'
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
	TS2554  = "TS2554"  // Expected N arguments, but got M
	TS2564  = "TS2564"  // Property 'X' has no initializer and is not definitely assigned in the constructor
	TS18013 = "TS18013" // Property 'X' is not accessible outside class 'Y'
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
