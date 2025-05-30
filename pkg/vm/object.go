package vm

import (
	"sort"
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

// GetOwn looks up a direct (own) property by name. Returns (value, true) if present.
func (o *PlainObject) GetOwn(name string) (Value, bool) {
	// Scan shape for the field
	for _, f := range o.shape.fields {
		if f.name == name {
			// matching field, properties slice must have value at offset
			if f.offset < len(o.properties) {
				return o.properties[f.offset], true
			}
			// absent value
			return Undefined, true
		}
	}
	return Undefined, false
}

// SetOwn sets or defines an own property. Creates a new shape on first definition.
func (o *PlainObject) SetOwn(name string, v Value) {
	// try to find existing field
	for _, f := range o.shape.fields {
		if f.name == name {
			// existing property, overwrite value
			o.properties[f.offset] = v
			return
		}
	}
	// new property: shape transition or creation
	cur := o.shape
	// reuse transition if exists
	next, ok := cur.transitions[name]
	if !ok {
		// create new shape by extending fields
		off := len(cur.fields)
		fld := Field{offset: off, name: name, writable: true, enumerable: true, configurable: true}
		// copy fields slice
		newFields := make([]Field, len(cur.fields)+1)
		copy(newFields, cur.fields)
		newFields[len(cur.fields)] = fld
		// new transitions map
		newTrans := make(map[string]*Shape)
		// assign new shape
		next = &Shape{parent: cur, fields: newFields, transitions: newTrans}
		// cache transition
		cur.transitions[name] = next
	}
	// adopt new shape
	o.shape = next
	// append value in properties slice
	o.properties = append(o.properties, v)
}

// HasOwn reports whether an own property with the given name exists.
func (o *PlainObject) HasOwn(name string) bool {
	for _, f := range o.shape.fields {
		if f.name == name {
			return true
		}
	}
	return false
}

// DeleteOwn deletes an own property. Not supported for PlainObject; always returns false.
func (o *PlainObject) DeleteOwn(name string) bool {
	// deletion of hidden-class properties not supported in this phase
	return false
}

// OwnKeys returns the list of own property names in insertion order.
func (o *PlainObject) OwnKeys() []string {
	keys := make([]string, len(o.shape.fields))
	for i, f := range o.shape.fields {
		keys[i] = f.name
	}
	return keys
}

type DictObject struct {
	Object
	prototype  Value
	properties map[string]Value
}

// GetOwn looks up a direct property by name. Returns (value, true) if present.
func (d *DictObject) GetOwn(name string) (Value, bool) {
	v, ok := d.properties[name]
	if !ok {
		return Undefined, false
	}
	return v, true
}

// SetOwn sets or defines an own property.
func (d *DictObject) SetOwn(name string, v Value) {
	d.properties[name] = v
}

// HasOwn reports whether an own property with the given name exists.
func (d *DictObject) HasOwn(name string) bool {
	_, ok := d.properties[name]
	return ok
}

// DeleteOwn deletes an own property. Returns true if deleted.
func (d *DictObject) DeleteOwn(name string) bool {
	if _, ok := d.properties[name]; ok {
		delete(d.properties, name)
		return true
	}
	return false
}

// OwnKeys returns the sorted list of own property names.
func (d *DictObject) OwnKeys() []string {
	keys := make([]string, 0, len(d.properties))
	for k := range d.properties {
		keys = append(keys, k)
	}
	// sort for deterministic order
	sort.Strings(keys)
	return keys
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
