package checker

import (
	"github.com/nooga/paserati/pkg/errors"
	"github.com/nooga/paserati/pkg/parser"
	"github.com/nooga/paserati/pkg/types"
)

// reportBadSpreadArgument reports a spread argument in a call that is neither
// an array nor a tuple: TS2488 when it cannot be iterated at all, TS2556 when
// it is iterable but is not being passed to a rest parameter.
func (c *Checker) reportBadSpreadArgument(spread *parser.SpreadElement, t types.Type) {
	if c.isDefinitelyNotIterable(t) {
		c.reportNotIterable(spread.Argument, t)
		return
	}
	if _, isObject := types.GetEffectiveType(t).(*types.ObjectType); isObject && c.isSpreadableIterableType(t) {
		c.addErrorWithCode(spread, errors.TS2556, "A spread argument must either have a tuple type or be passed to a rest parameter.")
	}
}
