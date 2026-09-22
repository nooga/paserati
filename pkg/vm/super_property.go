package vm

import "strconv"

// super property access: super.x, super[key], super.x = v, super[key] = v.
//
// A Super Reference's base is the home object's [[Prototype]] and its this
// value is the method's receiver. GetValue on one is base.[[Get]](key, this)
// and PutValue is base.[[Set]](key, v, this) (ECMAScript 6.2.5.5/6.2.5.6) -
// ordinary property access that merely starts one level up and keeps `this`
// as the receiver. The super opcodes used to reimplement that inline, with
// gaps: a symbol key was looked up by its "Symbol(desc)" string
// (paserati#518), and an accessor was only honoured on the immediate base
// (or, for static methods, only on the parent constructor itself), so a
// getter or setter inherited from further up read as undefined or was
// shadowed by a data property on `this`. Every super opcode now goes through
// superGet / superSetInLoop.

// superGet is base.[[Get]](key, thisValue). A getter anywhere on base's
// chain runs with this = thisValue.
func (vm *VM) superGet(base Value, key PropertyKey, thisValue Value) (Value, error) {
	if base.Type() == TypeNull || base.Type() == TypeUndefined {
		return Undefined, nil
	}
	if key.isSymbol() {
		return vm.getSymbolPropertyWithReceiver(base, key.symbolVal, thisValue)
	}
	return vm.getPropertyWithReceiver(base, key.name, thisValue)
}

// superSetFailure says why base.[[Set]] returned false; the zero value means
// it succeeded.
type superSetFailure uint8

const (
	superSetOK superSetFailure = iota
	superSetReadOnly
	superSetNotExtensible
)

// superSet is OrdinarySet(base, key, v, thisValue) (ECMAScript 10.1.9.2). A
// setter found anywhere on base's chain runs with this = thisValue; a
// writable data property (or none at all) makes the write land on
// thisValue itself. The chain is walked through each level's own property
// table, the same tables symbolPropsAndProto exposes for symbol lookups;
// a level with no table of its own (a Proxy, an Array used as a
// [[Prototype]]) ends the walk as if nothing were found there.
func (vm *VM) superSet(base Value, key PropertyKey, thisValue Value, v Value) (superSetFailure, error) {
	current := base
	// Same depth cap as findSymbolSlot, for the same reason.
	for i := 0; i < 100; i++ {
		tables, proto, ok := vm.symbolPropsAndProto(current)
		if !ok {
			break
		}
		for _, props := range tables {
			if props == nil {
				continue
			}
			if _, setter, _, _, isAcc := props.GetOwnAccessorByKey(key); isAcc {
				if setter.Type() == TypeUndefined {
					return superSetReadOnly, nil
				}
				_, err := vm.Call(setter, thisValue, []Value{v})
				return superSetOK, err
			}
			if _, writable, _, _, exists := props.GetOwnDescriptorByKey(key); exists {
				if !writable {
					return superSetReadOnly, nil
				}
				return vm.superDefineOnReceiver(thisValue, key, v), nil
			}
		}
		if proto.Type() == TypeUndefined || proto.Type() == TypeNull || proto == current {
			break
		}
		current = proto
	}
	return vm.superDefineOnReceiver(thisValue, key, v), nil
}

// superDefineOnReceiver is the receiver half of OrdinarySet (10.1.9.2 step
// 2): update an existing own writable data property, or create a new
// enumerable/writable/configurable one on an extensible receiver. A
// primitive receiver (possible in sloppy object-literal methods) can't take
// a property.
func (vm *VM) superDefineOnReceiver(receiver Value, key PropertyKey, v Value) superSetFailure {
	if receiver.Type() == TypeArray {
		return superDefineOnArray(receiver.AsArray(), key, v)
	}

	var props *PlainObject
	if receiver.Type() == TypeObject {
		props = receiver.AsPlainObject()
	} else {
		props = EnsureOwnPropertiesTable(receiver)
	}
	if props == nil {
		return superSetReadOnly
	}
	if _, _, _, _, isAcc := props.GetOwnAccessorByKey(key); isAcc {
		return superSetReadOnly
	}
	if _, writable, _, _, exists := props.GetOwnDescriptorByKey(key); exists {
		if !writable {
			return superSetReadOnly
		}
		props.DefineOwnPropertyByKey(key, v, nil, nil, nil)
		return superSetOK
	}
	if !props.IsExtensible() {
		return superSetNotExtensible
	}
	w, e, c := true, true, true
	props.DefineOwnPropertyByKey(key, v, &w, &e, &c)
	return superSetOK
}

// superDefineOnArray is superDefineOnReceiver for an Array receiver (an
// instance of a class extending Array): elements, length and symbol keys
// live outside the Properties side table.
func superDefineOnArray(arr *ArrayObject, key PropertyKey, v Value) superSetFailure {
	if arr == nil {
		return superSetReadOnly
	}
	if key.isSymbol() {
		symObj := key.symbolVal.AsSymbolObject()
		if symObj == nil {
			return superSetReadOnly
		}
		if _, desc, exists := arr.GetSymbolPropertyDescriptor(symObj); exists {
			if !desc.Writable {
				return superSetReadOnly
			}
		} else if !arr.IsExtensible() {
			return superSetNotExtensible
		}
		arr.SetSymbolProp(symObj, v)
		return superSetOK
	}
	if key.name == "length" {
		n := v.ToFloat()
		if n != n || n < 0 {
			n = 0
		}
		arr.SetLength(int(n))
		return superSetOK
	}
	if idx, err := strconv.Atoi(key.name); err == nil && idx >= 0 && idx < maxDenseSuperSetIndex {
		if idx >= arr.Length() && !arr.IsExtensible() {
			return superSetNotExtensible
		}
		arr.Set(idx, v)
		return superSetOK
	}
	if _, exists := arr.GetOwn(key.name); !exists && !arr.IsExtensible() {
		return superSetNotExtensible
	}
	arr.SetOwn(key.name, v)
	return superSetOK
}

// maxDenseSuperSetIndex bounds the element index superDefineOnArray writes
// densely: ArrayObject.Set fills every slot up to idx, so a key like
// "4294967294" would allocate billions of holes. Past it the key is stored
// as a named property, the same trade-off Object.assign makes.
const maxDenseSuperSetIndex = 1 << 24

// superSetInLoop runs superSet for the super-set opcodes and throws what it
// reports: a setter's exception, or - in strict code - the TypeError a
// false [[Set]] demands (PutValue step 5.b). It returns false when an
// exception is now in flight, in which case the caller resyncs its cached
// frame state.
func (vm *VM) superSetInLoop(frame *CallFrame, ip int, base Value, key PropertyKey, thisValue Value, v Value) bool {
	frame.ip = ip
	if base.Type() == TypeNull || base.Type() == TypeUndefined {
		vm.ThrowTypeError("Cannot set super property on " + base.Type().String() + " prototype")
		return false
	}
	failure, err := vm.superSet(base, key, thisValue, v)
	if err != nil {
		vm.throwFromCallError(err)
		return false
	}
	if failure == superSetOK {
		return true
	}
	strict := frame.closure != nil && frame.closure.Fn != nil && frame.closure.Fn.Chunk != nil && frame.closure.Fn.Chunk.IsStrict
	if !strict {
		return true
	}
	if failure == superSetNotExtensible {
		vm.ThrowTypeError("Cannot add property '" + key.debugName() + "', object is not extensible")
	} else {
		vm.ThrowTypeError("Cannot assign to read only property '" + key.debugName() + "' of object")
	}
	return false
}

// superGetInLoop runs superGet for the super-get opcodes, storing the result
// in *dest. It returns false when the getter threw, in which case the
// caller resyncs its cached frame state.
func (vm *VM) superGetInLoop(frame *CallFrame, ip int, base Value, key PropertyKey, thisValue Value, dest *Value) bool {
	frame.ip = ip
	result, err := vm.superGet(base, key, thisValue)
	if err != nil {
		vm.throwFromCallError(err)
		return false
	}
	*dest = result
	return true
}
