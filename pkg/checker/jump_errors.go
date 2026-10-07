package checker

import (
	"github.com/nooga/paserati/pkg/errors"
	"github.com/nooga/paserati/pkg/parser"
)

// reportBadJump reports a break or continue that has no valid target. When the
// only candidate target lies outside the enclosing function TypeScript says the
// jump crosses a function boundary (TS1107); otherwise the message depends on
// whether the statement is a break or a continue and whether it names a label.
func (c *Checker) reportBadJump(node parser.Node, isBreak, hasLabel bool) {
	switch {
	case c.crossFunctionTargets:
		c.addErrorWithCode(node, errors.TS1107, "Jump target cannot cross function boundary.")
	case isBreak && hasLabel:
		c.addErrorWithCode(node, errors.TS1116, "A 'break' statement can only jump to a label of an enclosing statement.")
	case isBreak:
		c.addErrorWithCode(node, errors.TS1105, "A 'break' statement can only be used within an enclosing iteration or switch statement.")
	case hasLabel:
		c.addErrorWithCode(node, errors.TS1115, "A 'continue' statement can only jump to a label of an enclosing iteration statement.")
	default:
		c.addErrorWithCode(node, errors.TS1104, "A 'continue' statement can only be used within an enclosing iteration statement.")
	}
}
