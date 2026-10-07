package errors

// TypeScript diagnostic codes reported by the assignability / type relation
// checks (kept apart from tscodes.go so relation work does not collide with
// other diagnostic additions).
const (
	TS2353 = "TS2353" // Object literal may only specify known properties, and 'X' does not exist in type 'Y'
	TS2403 = "TS2403" // Subsequent variable declarations must have the same type
	TS2415 = "TS2415" // Class 'X' incorrectly extends base class 'Y'
	TS2416 = "TS2416" // Property 'X' in type 'A' is not assignable to the same property in base type 'B'
	TS2430 = "TS2430" // Interface 'X' incorrectly extends interface 'Y'
	TS2559 = "TS2559" // Type 'X' has no properties in common with type 'Y'
	TS2739 = "TS2739" // Type 'X' is missing the following properties from type 'Y': a, b
	TS2740 = "TS2740" // Type 'X' is missing the following properties from type 'Y': a, b, c, d, and N more
	TS2741 = "TS2741" // Property 'X' is missing in type 'A' but required in type 'B'
)
