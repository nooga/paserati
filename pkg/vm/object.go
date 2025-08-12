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
	isAccessor   bool
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
	getters    map[string]Value
	setters    map[string]Value
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

// GetOwnDescriptor returns the value and attribute flags for an own property.
// Returns (value, writable, enumerable, configurable, exists).
func (o *PlainObject) GetOwnDescriptor(name string) (Value, bool, bool, bool, bool) {
	for _, f := range o.shape.fields {
		if f.name == name {
			if f.isAccessor {
				// For accessor, return undefined value and writable=false
				return Undefined, false, f.enumerable, f.configurable, true
			}
			var v Value = Undefined
			if f.offset < len(o.properties) {
				v = o.properties[f.offset]
			}
			return v, f.writable, f.enumerable, f.configurable, true
		}
	}
	return Undefined, false, false, false, false
}

// GetOwnAccessor returns accessor pair for an own property if it is an accessor.
// Returns (get, set, enumerable, configurable, exists)
func (o *PlainObject) GetOwnAccessor(name string) (Value, Value, bool, bool, bool) {
	for _, f := range o.shape.fields {
		if f.name == name && f.isAccessor {
			var g, s Value = Undefined, Undefined
			if o.getters != nil {
				if v, ok := o.getters[name]; ok {
					g = v
				}
			}
			if o.setters != nil {
				if v, ok := o.setters[name]; ok {
					s = v
				}
			}
			return g, s, f.enumerable, f.configurable, true
		}
	}
	return Undefined, Undefined, false, false, false
}

// DeleteOwn removes an own property if present and configurable.
// Returns true if the property was deleted.
func (o *PlainObject) DeleteOwn(name string) bool {
	// Find field index
	idx := -1
	var f Field
	for i := range o.shape.fields {
		if o.shape.fields[i].name == name {
			idx = i
			f = o.shape.fields[i]
			break
		}
	}
	if idx == -1 {
		// Non-existent own property: delete returns true per spec
		return true
	}
	// If not configurable, cannot delete
	if !f.configurable {
		return false
	}
	// Build new fields slice without idx
	newFields := make([]Field, 0, len(o.shape.fields)-1)
	for i, fld := range o.shape.fields {
		if i == idx {
			continue
		}
		// Adjust offsets and append
		nf := fld
		if fld.offset > f.offset {
			nf.offset = fld.offset - 1
		}
		newFields = append(newFields, nf)
	}
	// Build new properties slice without f.offset
	newProps := make([]Value, 0, len(o.properties)-1)
	for i := range o.properties {
		if i == f.offset {
			continue
		}
		newProps = append(newProps, o.properties[i])
	}
	// Create new shape without transitions for simplicity
	o.shape = &Shape{parent: o.shape.parent, fields: newFields, transitions: make(map[string]*Shape)}
	o.properties = newProps
	return true
}

// SetOwn sets or defines an own property. Creates a new shape on first definition.
// If the property exists and is non-writable, this is a no-op.
func (o *PlainObject) SetOwn(name string, v Value) {
	//fmt.Printf("DEBUG PlainObject.SetOwn: name=%q, value=%v, shape=%p\n", name, v.Inspect(), o.shape)
	// try to find existing field
	for _, f := range o.shape.fields {
		if f.name == name {
			// existing property, overwrite value
			// If not writable, ignore the set (non-strict mode semantics)
			if f.writable {
				o.properties[f.offset] = v
			}
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
		// Default attributes for regular assignment are writable/enumerable/configurable = true
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

// DefineOwnProperty defines or updates an own property with explicit attributes.
// For existing properties, unspecified attributes (nil) will keep previous values.
func (o *PlainObject) DefineOwnProperty(name string, value Value, writable *bool, enumerable *bool, configurable *bool) {
	// Try to find existing field
	for i, f := range o.shape.fields {
		if f.name == name {
			// Existing property: update value and attributes respecting current flags
			if f.isAccessor {
				// Converting accessor to data property: replace field
				f.isAccessor = false
				f.writable = false
			}
			// If writable is specified and current is false while new is true/false, allow toggle only if configurable is true
			// Minimal semantics: do not allow changing value when current writable is false
			if f.writable {
				o.properties[f.offset] = value
			}
			// Update flags if provided
			newF := f
			if writable != nil {
				newF.writable = *writable
			}
			if enumerable != nil {
				newF.enumerable = *enumerable
			}
			if configurable != nil {
				newF.configurable = *configurable
			}
			// Write back updated field in shape (copy-on-write not implemented; mutate in place)
			o.shape.fields[i] = newF
			return
		}
	}

	// New property: create shape transition with specified attributes (defaults false if nil)
	cur := o.shape
	cur.mu.RLock()
	next, ok := cur.transitions[name]
	cur.mu.RUnlock()
	if !ok {
		off := len(cur.fields)
		// Defaults per defineProperty: false when creating via descriptor if not specified
		fld := Field{offset: off, name: name, writable: false, enumerable: false, configurable: false, isAccessor: false}
		if writable != nil {
			fld.writable = *writable
		}
		if enumerable != nil {
			fld.enumerable = *enumerable
		}
		if configurable != nil {
			fld.configurable = *configurable
		}
		newFields := make([]Field, len(cur.fields)+1)
		copy(newFields, cur.fields)
		newFields[len(cur.fields)] = fld
		newTrans := make(map[string]*Shape)
		next = &Shape{parent: cur, fields: newFields, transitions: newTrans}
		cur.mu.Lock()
		if existing, exists := cur.transitions[name]; exists {
			next = existing
		} else {
			cur.transitions[name] = next
		}
		cur.mu.Unlock()
	}
	o.shape = next
	o.properties = append(o.properties, value)
}

// DefineAccessorProperty defines or updates an accessor own property.
func (o *PlainObject) DefineAccessorProperty(name string, getter Value, hasGetter bool, setter Value, hasSetter bool, enumerable *bool, configurable *bool) {
	// Find existing field
	for i, f := range o.shape.fields {
		if f.name == name {
			// Update to accessor kind
			newF := f
			newF.isAccessor = true
			// writable is meaningless for accessor
			if enumerable != nil {
				newF.enumerable = *enumerable
			}
			if configurable != nil {
				newF.configurable = *configurable
			}
			o.shape.fields[i] = newF
			if o.getters == nil {
				o.getters = make(map[string]Value)
			}
			if o.setters == nil {
				o.setters = make(map[string]Value)
			}
			if hasGetter {
				o.getters[name] = getter
			}
			if hasSetter {
				o.setters[name] = setter
			}
			return
		}
	}
	// New field
	cur := o.shape
	cur.mu.RLock()
	next, ok := cur.transitions[name]
	cur.mu.RUnlock()
	if !ok {
		off := len(cur.fields)
		fld := Field{offset: off, name: name, writable: false, enumerable: false, configurable: false, isAccessor: true}
		if enumerable != nil {
			fld.enumerable = *enumerable
		}
		if configurable != nil {
			fld.configurable = *configurable
		}
		newFields := make([]Field, len(cur.fields)+1)
		copy(newFields, cur.fields)
		newFields[len(cur.fields)] = fld
		newTrans := make(map[string]*Shape)
		next = &Shape{parent: cur, fields: newFields, transitions: newTrans}
		cur.mu.Lock()
		if existing, exists := cur.transitions[name]; exists {
			next = existing
		} else {
			cur.transitions[name] = next
		}
		cur.mu.Unlock()
	}
	o.shape = next
	// Ensure maps
	if o.getters == nil {
		o.getters = make(map[string]Value)
	}
	if o.setters == nil {
		o.setters = make(map[string]Value)
	}
	if hasGetter {
		o.getters[name] = getter
	}
	if hasSetter {
		o.setters[name] = setter
	}
	// Keep properties slice length consistent
	o.properties = append(o.properties, Undefined)
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
// (removed old stub DeleteOwn; implemented above)

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

// GetOwnDescriptor for DictObject returns default data property attributes (true, true, true) if present.
func (d *DictObject) GetOwnDescriptor(name string) (Value, bool, bool, bool, bool) {
	if v, ok := d.properties[name]; ok {
		return v, true, true, true, true
	}
	return Undefined, false, false, false, false
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
