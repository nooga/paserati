package parser

// VarDeclaredNames returns the names bound by `var` declarations anywhere in
// stmts, looking through nested blocks, loops, try/switch and labels but not
// into nested functions or classes. These are the names a function body (or
// script) hoists to its own scope. Duplicates are removed, first occurrence
// order is kept.
func VarDeclaredNames(stmts []Statement) []string {
	var scoped []scopedName
	for _, stmt := range stmts {
		if stmt == nil {
			continue
		}
		scoped = collectVarNames(stmt, scoped)
	}
	seen := make(map[string]bool, len(scoped))
	names := make([]string, 0, len(scoped))
	for _, n := range scoped {
		if !seen[n.name] {
			seen[n.name] = true
			names = append(names, n.name)
		}
	}
	return names
}
