package checker

import "github.com/nooga/paserati/pkg/parser"

// visitRemainingArguments type-checks the arguments from index start onward
// without matching them against a parameter list. It is used for rest
// parameters whose type is not a plain array (tuples, type parameters, unions of
// tuples), where we cannot yet match arguments one by one but must still check
// the argument expressions themselves.
func (c *Checker) visitRemainingArguments(arguments []parser.Expression, start int) {
	for i := start; i < len(arguments); i++ {
		if spread, ok := arguments[i].(*parser.SpreadElement); ok {
			c.visit(spread.Argument)
			continue
		}
		c.visit(arguments[i])
	}
}
