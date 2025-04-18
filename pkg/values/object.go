package values

import (
	"unsafe"
)

type Field struct {
	offset       int
	name         string
	writable     bool
	enumerable   bool
	configurable bool
}

type Shape struct {
	parent      *Shape
	fields      []Field
	transitions map[string]*Shape
}

type Object struct {
}

type PlainObject struct {
	Object
	shape      *Shape
	prototype  Value
	properties []Value
}

type DictObject struct {
	Object
	prototype  Value
	properties map[string]Value
}

// Define the shared default prototype for plain objects
var DefaultObjectPrototype Value
var RootShape *Shape

// Initialize the DefaultObjectPrototype once at package initialization
func init() {
	// The default prototype is an object whose own prototype is Null.
	protoObj := &PlainObject{prototype: Null}
	DefaultObjectPrototype = Value{typ: TypeObject, obj: unsafe.Pointer(protoObj)}
	RootShape = &Shape{
		fields:      []Field{},
		transitions: make(map[string]*Shape),
	}
}

func NewObject(proto Value) Value {
	// Create a new PlainObject and set its prototype to the shared DefaultObjectPrototype
	prototype := DefaultObjectPrototype
	if proto.IsObject() {
		prototype = proto
	}
	plainObj := &PlainObject{prototype: prototype, shape: RootShape}
	return Value{typ: TypeObject, obj: unsafe.Pointer(plainObj)}
}

func NewDictObject(proto Value) Value {
	prototype := DefaultObjectPrototype
	if proto.IsObject() {
		prototype = proto
	}
	dictObj := &DictObject{prototype: prototype, properties: make(map[string]Value)}
	return Value{typ: TypeDictObject, obj: unsafe.Pointer(dictObj)}
}
