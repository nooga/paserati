package compiler

import (
	"fmt"
	"math"

	"github.com/nooga/paserati/pkg/errors"
	"github.com/nooga/paserati/pkg/parser"
	"github.com/nooga/paserati/pkg/vm"
)

// compiledEnum is the enum object under construction for one scope+name. A
// merged enum (several `enum E {...}` declarations in one scope) keeps adding
// to the same object.
type compiledEnum struct {
	obj    vm.Value
	dict   *vm.DictObject
	consts map[string]*parser.EnumConst // nil entry: computed member
}

type enumRegistryKey struct {
	scope *SymbolTable
	name  string
}

// compileEnumDeclaration compiles an enum declaration to bytecode.
//
// Constant members are folded at compile time (literals, operators, and
// references to earlier members - tsc's constant evaluator, shared with the
// checker through parser.EvalEnumConst) and written straight into the enum
// object. A computed member (`A = "foo".length`) is evaluated at run time and
// stored together with its reverse mapping, with earlier members visible by
// name inside its initializer.
func (c *Compiler) compileEnumDeclaration(node *parser.EnumDeclaration, hint Register) (Register, errors.PaseratiError) {
	debugPrintf("// [Compiler Enum] Compiling enum declaration '%s'\n", node.Name.Value)
	enumName := node.Name.Value

	if c.enumRegistry == nil {
		c.enumRegistry = make(map[enumRegistryKey]*compiledEnum)
	}
	regKey := enumRegistryKey{scope: c.currentSymbolTable, name: enumName}
	ce := c.enumRegistry[regKey]
	if ce == nil {
		obj := vm.NewDictObject(vm.DefaultObjectPrototype)
		ce = &compiledEnum{obj: obj, dict: obj.AsDictObject(), consts: make(map[string]*parser.EnumConst)}
		c.enumRegistry[regKey] = ce
	}

	// Install the enum object first so run-time initializers can see it.
	enumConstIndex := c.chunk.AddConstant(ce.obj)
	c.emitLoadConstant(hint, enumConstIndex, node.Token.Line)
	// Namespace the heap key the same way top-level classes/functions/vars do
	// (see moduleGlobalKey, #103/#106): an enum always installs a global here
	// regardless of nesting, so an un-namespaced key would leak across
	// modules exactly like the class case #103 first found.
	globalIdx := c.GetOrAssignGlobalIndex(c.moduleGlobalKey(enumName))
	c.currentSymbolTable.DefineGlobal(enumName, globalIdx)
	c.emitSetGlobal(globalIdx, hint, node.Token.Line)

	nextValue := 0.0
	autoValid := true // false after a computed or string member
	declared := make(map[string]bool, len(node.Members))

	for _, member := range node.Members {
		if member == nil || member.Name == nil {
			continue
		}
		memberName := member.Name.Value
		if _, dup := ce.consts[memberName]; dup {
			continue // duplicate member: reported by the checker; first wins
		}

		resolve := func(qualifier, name string) (parser.EnumConst, bool) {
			if qualifier != "" && qualifier != enumName {
				if other := c.enumRegistry[enumRegistryKey{scope: c.currentSymbolTable, name: qualifier}]; other != nil {
					if v := other.consts[name]; v != nil {
						return *v, true
					}
				}
				return parser.EnumConst{}, false
			}
			if v := ce.consts[name]; v != nil {
				return *v, true
			}
			return parser.EnumConst{}, false
		}

		var constVal *parser.EnumConst
		if member.Value != nil {
			if v, ok := parser.EvalEnumConst(member.Value, resolve); ok {
				constVal = &v
			}
		} else {
			if !autoValid {
				return BadRegister, NewCompileError(member.Name, "enum member must have initializer")
			}
			constVal = &parser.EnumConst{Num: nextValue}
		}

		if constVal != nil {
			ce.consts[memberName] = constVal
			declared[memberName] = true
			if constVal.IsString {
				ce.dict.SetOwn(memberName, vm.String(constVal.Str))
				autoValid = false
			} else {
				ce.dict.SetOwn(memberName, vm.Number(constVal.Num))
				ce.dict.SetOwn(enumReverseKey(constVal.Num), vm.String(memberName))
				nextValue = constVal.Num + 1
				autoValid = true
			}
			continue
		}

		// Computed member: evaluated when the declaration runs.
		if err := c.compileComputedEnumMember(node, ce, member, hint); err != nil {
			return BadRegister, err
		}
		ce.consts[memberName] = nil
		declared[memberName] = true
		autoValid = false
	}

	debugPrintf("// [Compiler Enum] Successfully compiled enum '%s' with %d members\n", enumName, len(node.Members))
	return hint, nil
}

// compileComputedEnumMember emits `E["name"] = <init>; E[E["name"]] = "name"`
// with the members the initializer mentions bound by name.
func (c *Compiler) compileComputedEnumMember(node *parser.EnumDeclaration, ce *compiledEnum, member *parser.EnumMember, enumReg Register) errors.PaseratiError {
	line := node.Token.Line
	memberName := member.Name.Value

	prevTable := c.currentSymbolTable
	c.currentSymbolTable = NewEnclosedSymbolTable(prevTable)
	var temps []Register
	defer func() {
		c.currentSymbolTable = prevTable
		for _, r := range temps {
			c.regAlloc.Free(r)
		}
	}()

	for _, name := range parser.ReferencedIdentifiers(member.Value) {
		v, known := ce.consts[name]
		if !known {
			continue
		}
		reg := c.regAlloc.Alloc()
		temps = append(temps, reg)
		if v != nil {
			if v.IsString {
				c.emitLoadConstant(reg, c.chunk.AddConstant(vm.String(v.Str)), line)
			} else {
				c.emitLoadConstant(reg, c.chunk.AddConstant(vm.Number(v.Num)), line)
			}
		} else {
			c.emitGetProp(reg, enumReg, uint16(c.chunk.AddConstant(vm.String(name))), line)
		}
		c.currentSymbolTable.Define(name, reg)
	}

	valueReg := c.regAlloc.Alloc()
	temps = append(temps, valueReg)
	if _, err := c.compileNode(member.Value, valueReg); err != nil {
		return err
	}
	nameConst := uint16(c.chunk.AddConstant(vm.String(memberName)))
	c.emitSetProp(enumReg, valueReg, nameConst, line)

	// Reverse mapping: E[value] = "name".
	nameReg := c.regAlloc.Alloc()
	temps = append(temps, nameReg)
	c.emitLoadConstant(nameReg, uint16(nameConst), line)
	c.emitOpCode(vm.OpSetIndex, line)
	c.emitByte(byte(enumReg))
	c.emitByte(byte(valueReg))
	c.emitByte(byte(nameReg))
	return nil
}

// enumReverseKey is the property key of a numeric member's reverse mapping.
func enumReverseKey(v float64) string {
	if v == math.Trunc(v) && math.Abs(v) < 1e21 {
		return fmt.Sprintf("%d", int64(v))
	}
	return vm.Number(v).ToString()
}
