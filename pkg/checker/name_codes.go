package checker

// TypeScript diagnostic codes used by name resolution / declaration checks.
// Kept as package-local constants (rather than in pkg/errors/tscodes.go) so
// this file never conflicts with other work extending the shared table.
const (
	tsDuplicateIdentifier      = "TS2300" // Duplicate identifier 'X'.
	tsBlockScopedRedeclaration = "TS2451" // Cannot redeclare block-scoped variable 'X'.
	tsSubsequentVarDecl        = "TS2403" // Subsequent variable declarations must have the same type.
	tsDuplicateFunctionImpl    = "TS2393" // Duplicate function implementation.
	tsBlockScopedUsedBefore    = "TS2448" // Block-scoped variable 'X' used before its declaration.
	tsClassUsedBefore          = "TS2449" // Class 'X' used before its declaration.
	tsEnumUsedBefore           = "TS2450" // Enum 'X' used before its declaration.
	tsValueUsedAsType          = "TS2749" // 'X' refers to a value, but is being used as a type here.
)

// redeclarationReportedByBinder marks the places where an Environment.Define
// collision used to report "already declared". Duplicate declarations are
// reported once, with TypeScript's codes and merge rules, by the parser's
// declaration binder (Program.BindErrors); the collision itself only means the
// later declaration does not replace the earlier binding.
func (c *Checker) redeclarationReportedByBinder() {}
