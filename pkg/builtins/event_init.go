package builtins

import (
	"time"

	"github.com/nooga/paserati/pkg/types"
	"github.com/nooga/paserati/pkg/vm"
)

// The DOM Event interface (https://dom.spec.whatwg.org/#interface-event).
// Built-in dispatchers such as AbortSignal create their events from it, so
// listeners receive real `Event` instances (#567). A host that ships its own
// Event can still replace the global; built-in events keep using this one.

// Event phases (Event.NONE ... Event.BUBBLING_PHASE).
const (
	eventPhaseNone      = 0
	eventPhaseCapturing = 1
	eventPhaseAtTarget  = 2
	eventPhaseBubbling  = 3
)

// eventSlots is an Event instance's internal state.
type eventSlots struct {
	typ           string
	target        vm.Value
	currentTarget vm.Value
	phase         int
	bubbles       bool
	cancelable    bool
	composed      bool
	trusted       bool
	timeStamp     float64

	canceled      bool // "canceled flag", read back as defaultPrevented
	stopProp      bool
	stopImmediate bool
	inPassive     bool
	dispatching   bool
}

// eventRealm holds one VM's Event intrinsics.
type eventRealm struct {
	vm        *vm.VM
	proto     *vm.PlainObject
	isTrusted vm.Value // shared getter for the per-instance isTrusted property
}

// eventConstructorType is the static type of the Event global.
func eventConstructorType() *types.ObjectType {
	eventType := types.NewObjectType().
		WithProperty("type", types.String).
		WithProperty("target", types.Any).
		WithProperty("srcElement", types.Any).
		WithProperty("currentTarget", types.Any).
		WithProperty("eventPhase", types.Number).
		WithProperty("bubbles", types.Boolean).
		WithProperty("cancelable", types.Boolean).
		WithProperty("defaultPrevented", types.Boolean).
		WithProperty("composed", types.Boolean).
		WithProperty("isTrusted", types.Boolean).
		WithProperty("timeStamp", types.Number).
		WithProperty("cancelBubble", types.Boolean).
		WithProperty("returnValue", types.Boolean).
		WithProperty("composedPath", types.NewSimpleFunction([]types.Type{}, &types.ArrayType{ElementType: types.Any})).
		WithProperty("preventDefault", types.NewSimpleFunction([]types.Type{}, types.Undefined)).
		WithProperty("stopPropagation", types.NewSimpleFunction([]types.Type{}, types.Undefined)).
		WithProperty("stopImmediatePropagation", types.NewSimpleFunction([]types.Type{}, types.Undefined)).
		WithProperty("initEvent", types.NewOptionalFunction([]types.Type{types.String, types.Boolean, types.Boolean}, types.Undefined, []bool{false, true, true}))
	ctorType := types.NewObjectType().
		WithConstructSignature(&types.Signature{
			ParameterTypes: []types.Type{types.String, types.Any},
			ReturnType:     eventType,
			OptionalParams: []bool{false, true},
		}).
		WithProperty("prototype", eventType)
	for _, name := range []string{"NONE", "CAPTURING_PHASE", "AT_TARGET", "BUBBLING_PHASE"} {
		eventType = eventType.WithProperty(name, types.Number)
		ctorType = ctorType.WithProperty(name, types.Number)
	}
	return ctorType
}

// installEvent builds Event and Event.prototype and defines the Event global.
func installEvent(vmi *vm.VM, ctx *RuntimeContext) (*eventRealm, error) {
	proto := vm.NewObject(vmi.ObjectPrototype).AsPlainObject()
	protoVal := vm.NewValueFromPlainObject(proto)
	r := &eventRealm{vm: vmi, proto: proto}

	ctor := vm.NewConstructorWithProps(1, false, "Event", func(args []vm.Value) (vm.Value, error) {
		newTarget := vmi.GetNewTarget()
		if newTarget.IsUndefined() {
			return vm.Undefined, vmi.NewTypeError("Failed to construct 'Event': Please use the 'new' operator, this DOM object constructor cannot be called as a function.")
		}
		if len(args) < 1 {
			return vm.Undefined, vmi.NewTypeError("Failed to construct 'Event': 1 argument required, but only 0 present.")
		}
		typ, err := getStringValueWithVM(vmi, args[0])
		if err != nil {
			return vm.Undefined, err
		}
		s := &eventSlots{typ: typ}
		if len(args) > 1 {
			if err := r.readEventInit(args[1], s); err != nil {
				return vm.Undefined, err
			}
		}
		// OrdinaryCreateFromConstructor: a subclass's prototype, else ours.
		p, err := vmi.GetProperty(newTarget, "prototype")
		if err != nil {
			return vm.Undefined, err
		}
		if !p.IsObject() {
			p = protoVal
		}
		return r.create(p, s), nil
	})
	ctorProps := ctor.AsNativeFunctionWithProps().Properties
	ctorProps.DefineFixedProperty("prototype", protoVal)
	proto.SetOwnNonEnumerable("constructor", ctor)

	for i, name := range []string{"NONE", "CAPTURING_PHASE", "AT_TARGET", "BUBBLING_PHASE"} {
		w, e, c := false, true, false
		ctorProps.DefineOwnProperty(name, vm.NumberValue(float64(i)), &w, &e, &c)
		proto.DefineOwnProperty(name, vm.NumberValue(float64(i)), &w, &e, &c)
	}

	// Attributes and operations, WebIDL-style: enumerable, configurable.
	getter := func(name string, get func(s *eventSlots) vm.Value) {
		r.accessor(name, get, nil)
	}
	getter("type", func(s *eventSlots) vm.Value { return vm.NewString(s.typ) })
	getter("target", func(s *eventSlots) vm.Value { return s.target })
	getter("srcElement", func(s *eventSlots) vm.Value { return s.target })
	getter("currentTarget", func(s *eventSlots) vm.Value { return s.currentTarget })
	getter("eventPhase", func(s *eventSlots) vm.Value { return vm.NumberValue(float64(s.phase)) })
	getter("bubbles", func(s *eventSlots) vm.Value { return vm.BooleanValue(s.bubbles) })
	getter("cancelable", func(s *eventSlots) vm.Value { return vm.BooleanValue(s.cancelable) })
	getter("defaultPrevented", func(s *eventSlots) vm.Value { return vm.BooleanValue(s.canceled) })
	getter("composed", func(s *eventSlots) vm.Value { return vm.BooleanValue(s.composed) })
	getter("timeStamp", func(s *eventSlots) vm.Value { return vm.NumberValue(s.timeStamp) })
	r.accessor("cancelBubble",
		func(s *eventSlots) vm.Value { return vm.BooleanValue(s.stopProp) },
		func(s *eventSlots, v vm.Value) {
			if v.IsTruthy() {
				s.stopProp = true
			}
		})
	r.accessor("returnValue",
		func(s *eventSlots) vm.Value { return vm.BooleanValue(!s.canceled) },
		func(s *eventSlots, v vm.Value) {
			if !v.IsTruthy() {
				s.setCanceled()
			}
		})

	r.method("composedPath", 0, func(s *eventSlots, _ []vm.Value) (vm.Value, error) {
		if s.currentTarget.Type() == vm.TypeNull {
			return vm.NewArray(), nil
		}
		return vm.NewArrayWithArgs([]vm.Value{s.currentTarget}), nil
	})
	r.method("stopPropagation", 0, func(s *eventSlots, _ []vm.Value) (vm.Value, error) {
		s.stopProp = true
		return vm.Undefined, nil
	})
	r.method("stopImmediatePropagation", 0, func(s *eventSlots, _ []vm.Value) (vm.Value, error) {
		s.stopProp = true
		s.stopImmediate = true
		return vm.Undefined, nil
	})
	r.method("preventDefault", 0, func(s *eventSlots, _ []vm.Value) (vm.Value, error) {
		s.setCanceled()
		return vm.Undefined, nil
	})
	r.method("initEvent", 1, func(s *eventSlots, args []vm.Value) (vm.Value, error) {
		if len(args) < 1 {
			return vm.Undefined, vmi.NewTypeError("Failed to execute 'initEvent' on 'Event': 1 argument required, but only 0 present.")
		}
		typ, err := getStringValueWithVM(vmi, args[0])
		if err != nil {
			return vm.Undefined, err
		}
		if s.dispatching {
			return vm.Undefined, nil
		}
		s.typ = typ
		s.bubbles = len(args) > 1 && args[1].IsTruthy()
		s.cancelable = len(args) > 2 && args[2].IsTruthy()
		s.stopProp, s.stopImmediate, s.canceled = false, false, false
		s.trusted = false
		s.target = vm.Null
		return vm.Undefined, nil
	})
	intlDefineToStringTag(vmi, proto, "Event")

	r.isTrusted = vm.NewNativeFunction(0, false, "get isTrusted", func(_ []vm.Value) (vm.Value, error) {
		s, err := r.slotsOf(vmi.GetThis())
		if err != nil {
			return vm.Undefined, err
		}
		return vm.BooleanValue(s.trusted), nil
	})

	return r, ctx.DefineGlobal("Event", ctor)
}

// readEventInit reads an EventInit dictionary into s.
func (r *eventRealm) readEventInit(dict vm.Value, s *eventSlots) error {
	if dict.Type() == vm.TypeUndefined || dict.Type() == vm.TypeNull {
		return nil
	}
	if !dict.IsObject() && !dict.IsCallable() {
		return r.vm.NewTypeError("Failed to construct 'Event': The provided value is not of type 'EventInit'.")
	}
	for _, f := range []struct {
		name string
		dst  *bool
	}{{"bubbles", &s.bubbles}, {"cancelable", &s.cancelable}, {"composed", &s.composed}} {
		v, err := r.vm.GetProperty(dict, f.name)
		if err != nil {
			return err
		}
		*f.dst = v.IsTruthy()
	}
	return nil
}

// create makes an Event object with prototype proto and state s.
func (r *eventRealm) create(proto vm.Value, s *eventSlots) vm.Value {
	s.target, s.currentTarget = vm.Null, vm.Null
	s.timeStamp = float64(time.Since(performanceOrigin).Nanoseconds()) / 1e6
	obj := vm.NewObject(proto).AsPlainObject()
	obj.SetInternalSlots(s)
	// isTrusted is [LegacyUnforgeable]: an own, non-configurable accessor.
	e, c := true, false
	obj.DefineAccessorProperty("isTrusted", r.isTrusted, true, vm.Undefined, false, &e, &c)
	return vm.NewValueFromPlainObject(obj)
}

// newTrustedEvent creates an event dispatched by the runtime itself.
func (r *eventRealm) newTrustedEvent(typ string) (vm.Value, *eventSlots) {
	s := &eventSlots{typ: typ, trusted: true}
	return r.create(vm.NewValueFromPlainObject(r.proto), s), s
}

func (r *eventRealm) slotsOf(v vm.Value) (*eventSlots, error) {
	if v.Type() == vm.TypeObject {
		if s, ok := v.AsPlainObject().InternalSlots().(*eventSlots); ok {
			return s, nil
		}
	}
	return nil, r.vm.NewTypeError("Illegal invocation")
}

func (r *eventRealm) accessor(name string, get func(*eventSlots) vm.Value, set func(*eventSlots, vm.Value)) {
	getFn := vm.NewNativeFunction(0, false, "get "+name, func(_ []vm.Value) (vm.Value, error) {
		s, err := r.slotsOf(r.vm.GetThis())
		if err != nil {
			return vm.Undefined, err
		}
		return get(s), nil
	})
	setFn := vm.Undefined
	if set != nil {
		setFn = vm.NewNativeFunction(1, false, "set "+name, func(args []vm.Value) (vm.Value, error) {
			s, err := r.slotsOf(r.vm.GetThis())
			if err != nil {
				return vm.Undefined, err
			}
			v := vm.Undefined
			if len(args) > 0 {
				v = args[0]
			}
			set(s, v)
			return vm.Undefined, nil
		})
	}
	e, c := true, true
	r.proto.DefineAccessorProperty(name, getFn, true, setFn, set != nil, &e, &c)
}

func (r *eventRealm) method(name string, arity int, fn func(*eventSlots, []vm.Value) (vm.Value, error)) {
	f := vm.NewNativeFunction(arity, false, name, func(args []vm.Value) (vm.Value, error) {
		s, err := r.slotsOf(r.vm.GetThis())
		if err != nil {
			return vm.Undefined, err
		}
		return fn(s, args)
	})
	w, e, c := true, true, true
	r.proto.DefineOwnProperty(name, f, &w, &e, &c)
}

// setCanceled is "set the canceled flag": only for a cancelable event outside
// a passive listener.
func (s *eventSlots) setCanceled() {
	if s.cancelable && !s.inPassive {
		s.canceled = true
	}
}

// beginDispatch and endDispatch bracket dispatching s at target (no tree:
// the event is only ever AT_TARGET).
func (s *eventSlots) beginDispatch(target vm.Value) {
	s.dispatching = true
	s.target, s.currentTarget = target, target
	s.phase = eventPhaseAtTarget
}

func (s *eventSlots) endDispatch() {
	s.dispatching = false
	s.currentTarget = vm.Null
	s.phase = eventPhaseNone
	s.stopProp, s.stopImmediate = false, false
}
