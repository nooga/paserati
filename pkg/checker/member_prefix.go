package checker

import (
	"fmt"

	"github.com/nooga/paserati/pkg/parser"
	"github.com/nooga/paserati/pkg/types"
)

// classMemberFrame is a class member body being checked.
type classMemberFrame struct {
	className string
	ctor      *types.ObjectType // the class's constructor type (static side)
	instance  *types.ObjectType
	static    bool
	// fnDepth is nonArrowFunctionDepth outside the member's function: a
	// use directly in the member (arrow functions aside) sees depth+1.
	fnDepth int
}

// pushMemberFrame records the method or constructor body about to be checked.
func (c *Checker) pushMemberFrame(ctx *types.AccessContext, thisType types.Type, instance *types.ObjectType) {
	frame := classMemberFrame{instance: instance, fnDepth: c.nonArrowFunctionDepth}
	if ctx != nil {
		frame.className = ctx.CurrentClassName
		frame.static = ctx.ContextType == types.AccessContextStaticMethod
	}
	if instance != nil && instance.GetClassName() != "" {
		frame.className = instance.GetClassName()
	}
	if frame.static {
		frame.ctor, _ = thisType.(*types.ObjectType)
	} else if frame.className != "" {
		if t, _, ok := c.env.Resolve(frame.className); ok {
			frame.ctor, _ = t.(*types.ObjectType)
		}
	}
	c.memberFrames = append(c.memberFrames, frame)
}

func (c *Checker) popMemberFrame() {
	c.memberFrames = c.memberFrames[:len(c.memberFrames)-1]
}

// reportMissingMemberPrefix mirrors tsc's checkAndReportErrorForMissingPrefix:
// an unresolved name inside a class member that names a static member of an
// enclosing class is TS2662, and one naming an instance member, used directly
// in an instance member, is TS2663.
func (c *Checker) reportMissingMemberPrefix(node *parser.Identifier) bool {
	name := node.Value
	for i := len(c.memberFrames) - 1; i >= 0; i-- {
		frame := c.memberFrames[i]
		if frame.className == "" {
			continue
		}
		if frame.ctor != nil {
			if _, ok := frame.ctor.GetEffectiveProperties()[name]; ok {
				c.addErrorWithCode(node, "TS2662", fmt.Sprintf("Cannot find name '%s'. Did you mean the static member '%s.%s'?", name, frame.className, name))
				return true
			}
		}
		if i == len(c.memberFrames)-1 && !frame.static && frame.instance != nil && c.nonArrowFunctionDepth == frame.fnDepth+1 {
			if _, ok := frame.instance.GetEffectiveProperties()[name]; ok {
				c.addErrorWithCode(node, "TS2663", fmt.Sprintf("Cannot find name '%s'. Did you mean the instance member 'this.%s'?", name, name))
				return true
			}
		}
	}
	return false
}
