package vm

import (
	"sort"
	"sync"
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
	mu          sync.RWMutex // Protects transitions map
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
	//fmt.Printf("DEBUG PlainObject.SetOwn: name=%q, value=%v, shape=%p\n", name, v.Inspect(), o.shape)
	// try to find existing field
	for _, f := range o.shape.fields {
		if f.name == name {
			// existing property, overwrite value
			//fmt.Printf("DEBUG PlainObject.SetOwn: Overwriting existing property %q\n", name)
			o.properties[f.offset] = v
			return
		}
	}
	// new property: shape transition or creation
	//fmt.Printf("DEBUG PlainObject.SetOwn: Adding new property %q\n", name)
	cur := o.shape
	// reuse transition if exists (with read lock)
	cur.mu.RLock()
	next, ok := cur.transitions[name]
	cur.mu.RUnlock()
	
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
		
		// cache transition (with write lock and double-check)
		cur.mu.Lock()
		if existing, exists := cur.transitions[name]; exists {
			// Another goroutine created the transition while we were working
			next = existing
		} else {
			cur.transitions[name] = next
		}
		cur.mu.Unlock()
	}
	// adopt new shape
	//fmt.Printf("DEBUG PlainObject.SetOwn: Shape transition: %p -> %p\n", o.shape, next)
	o.shape = next
	// append value in properties slice
	o.properties = append(o.properties, v)
	//fmt.Printf("DEBUG PlainObject.SetOwn: Property %q added, new shape has %d fields\n", name, len(o.shape.fields))
}

// HasOwn reports whether an own property with the given name exists.
func (o *PlainObject) HasOwn(name string) bool {
	//fmt.Printf("DEBUG PlainObject.HasOwn: name=%q, shape=%p, fields=%v\n", name, o.shape, o.shape.fields)
	for _, f := range o.shape.fields {
		//fmt.Printf("DEBUG PlainObject.HasOwn: field[%d]=%q\n", i, f.name)
		if f.name == name {
			//fmt.Printf("DEBUG PlainObject.HasOwn: found %q at index %d\n", name, i)
			return true
		}
	}
	//fmt.Printf("DEBUG PlainObject.HasOwn: %q not found\n", name)
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

// Get looks up a property by name, walking the prototype chain if necessary.
func (o *PlainObject) Get(name string) (Value, bool) {
	// Check own properties first
	if value, exists := o.GetOwn(name); exists {
		return value, true
	}

	// Walk prototype chain
	current := o.prototype
	for current.typ != TypeNull && current.typ != TypeUndefined {
		if current.IsObject() {
			if proto := current.AsPlainObject(); proto != nil {
				if value, exists := proto.GetOwn(name); exists {
					return value, true
				}
				current = proto.prototype
			} else if dict := current.AsDictObject(); dict != nil {
				if value, exists := dict.GetOwn(name); exists {
					return value, true
				}
				current = dict.prototype
			} else {
				break
			}
		} else {
			break
		}
	}

	return Undefined, false
}

// Has reports whether a property with the given name exists (own or inherited).
func (o *PlainObject) Has(name string) bool {
	_, exists := o.Get(name)
	return exists
}

// GetPrototype returns the object's prototype.
func (o *PlainObject) GetPrototype() Value {
	return o.prototype
}

// SetPrototype sets the object's prototype.
func (o *PlainObject) SetPrototype(proto Value) {
	o.prototype = proto
	// TODO: Invalidate related caches
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

// Get looks up a property by name, walking the prototype chain if necessary.
func (d *DictObject) Get(name string) (Value, bool) {
	// Check own properties first
	if value, exists := d.GetOwn(name); exists {
		return value, true
	}

	// Walk prototype chain
	current := d.prototype
	for current.typ != TypeNull && current.typ != TypeUndefined {
		if current.IsObject() {
			if proto := current.AsPlainObject(); proto != nil {
				if value, exists := proto.GetOwn(name); exists {
					return value, true
				}
				current = proto.prototype
			} else if dict := current.AsDictObject(); dict != nil {
				if value, exists := dict.GetOwn(name); exists {
					return value, true
				}
				current = dict.prototype
			} else {
				break
			}
		} else {
			break
		}
	}

	return Undefined, false
}

// Has reports whether a property with the given name exists (own or inherited).
func (d *DictObject) Has(name string) bool {
	_, exists := d.Get(name)
	return exists
}

// GetPrototype returns the object's prototype.
func (d *DictObject) GetPrototype() Value {
	return d.prototype
}

// SetPrototype sets the object's prototype.
func (d *DictObject) SetPrototype(proto Value) {
	d.prototype = proto
	// TODO: Invalidate related caches
}

// Define the shared default prototype for plain objects
var DefaultObjectPrototype Value
var RootShape *Shape

// Initialize the DefaultObjectPrototype once at package initialization
func init() {
	// Initialize RootShape first
	RootShape = &Shape{
		fields:      []Field{},
		transitions: make(map[string]*Shape),
	}
	// The default prototype is an object whose own prototype is Null.
	protoObj := &PlainObject{prototype: Null, shape: RootShape}
	DefaultObjectPrototype = Value{typ: TypeObject, obj: unsafe.Pointer(protoObj)}
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
