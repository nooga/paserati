package checker

import (
	"sort"

	"github.com/nooga/paserati/pkg/parser"
)

// hoistedEntry is one function declaration hoisted to the top of a program or
// block.
type hoistedEntry struct {
	name string
	node parser.Expression
}

// hoistedInSourceOrder returns the hoisted declarations of a program or block
// in source order. The parser keeps them in a map, and checking their
// signatures and bodies in map order made diagnostics differ from run to run:
// which function is checked first decides what later ones see (inferred
// return types, instantiation caches).
func hoistedInSourceOrder(hoisted map[string]parser.Expression) []hoistedEntry {
	entries := make([]hoistedEntry, 0, len(hoisted))
	for name, node := range hoisted {
		entries = append(entries, hoistedEntry{name, node})
	}
	sort.Slice(entries, func(i, j int) bool {
		pi, pj := hoistedPos(entries[i].node), hoistedPos(entries[j].node)
		if pi != pj {
			return pi < pj
		}
		return entries[i].name < entries[j].name
	})
	return entries
}

func hoistedPos(node parser.Expression) int {
	if fn, ok := node.(*parser.FunctionLiteral); ok && fn.Token != nil {
		return fn.Token.StartPos
	}
	return -1
}
