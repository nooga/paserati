package vm

import (
	"fmt"
	"sort"
	"sync"
	"unsafe"
)

type KeyKind uint8

const (
	KeyKindString KeyKind = iota
	KeyKindSymbol
	KeyKindPrivate // reserved for future private fields
)

// PropertyKey represents a property key which can be a string, symbol, or private key
type PropertyKey struct {
	kind      KeyKind
	name      string // for string keys
	symbolVal Value  // for symbol keys (TypeSymbol)
	// private identity reserved for future use
}

func keyFromString(name string) PropertyKey {
	return PropertyKey{kind: KeyKindString, name: name}
}

func keyFromSymbol(sym Value) PropertyKey {
	return PropertyKey{kind: KeyKindSymbol, symbolVal: sym}
}

// NewStringKey constructs an exported PropertyKey for string-named properties.
func NewStringKey(name string) PropertyKey { return keyFromString(name) }

// NewSymbolKey constructs an exported PropertyKey for symbol-named properties.
func NewSymbolKey(sym Value) PropertyKey { return keyFromSymbol(sym) }

func (k PropertyKey) isString() bool { return k.kind == KeyKindString }
func (k PropertyKey) isSymbol() bool { return k.kind == KeyKindSymbol }

func (k PropertyKey) debugName() string {
	switch k.kind {
	case KeyKindString:
		return k.name
	case KeyKindSymbol:
		return fmt.Sprintf("Symbol(%s)", k.symbolVal.AsSymbol())
	case KeyKindPrivate:
		return "<private>"
	default:
		return "<unknown-key>"
	}
}

func (k PropertyKey) hash() string {
	switch k.kind {
	case KeyKindString:
		return "s:" + k.name
	case KeyKindSymbol:
		return fmt.Sprintf("y:%p", k.symbolVal.obj)
	case KeyKindPrivate:
		return "p:todo" // placeholder until private identities are implemented
	default:
		return "?"
	}
}

type Field struct {
	offset int
	// For string keys, name holds the property name; for symbols, it may be empty (debug-only)
	name         string
	keyKind      KeyKind
	symbolVal    Value // valid when keyKind == KeyKindSymbol
	writable     bool
	enumerable   bool
	configurable bool
	isAccessor   bool
}

type Shape struct {
	parent      *Shape
	fields      []Field
	transitions map[string]*Shape // keyed by PropertyKey.hash()
	mu          sync.RWMutex      // Protects transitions map
	version     uint32            // Bumped on any layout/flags change
}

type Object struct {
}

type PlainObject struct {
	Object
	shape      *Shape
	prototype  Value
	properties []Value
	// Accessor storage keyed by PropertyKey.hash()
	getters map[string]Value
	setters map[string]Value
	// Private field storage (ECMAScript # fields)
	// Keyed by field name (without the # prefix)
	privateFields map[string]Value
	// Private methods storage (ECMAScript # methods)
	// Keyed by method name (without the # prefix)
	// Methods are not writable - assignment throws TypeError
	privateMethods map[string]Value
	// Private accessor storage (ECMAScript # getters/setters)
	// Keyed by field name (without the # prefix)
	privateGetters map[string]Value
	privateSetters map[string]Value
	// Extensible flag - when false, no new properties can be added
	extensible bool
}

// GetOwn looks up a direct (own) property by name. Returns (value, true) if present.
func (o *PlainObject) GetOwn(name string) (Value, bool) {
	return o.GetOwnByKey(keyFromString(name))
}

// GetOwnByKey looks up a direct (own) property by key. Returns (value, true) if present.
func (o *PlainObject) GetOwnByKey(key PropertyKey) (Value, bool) {
	// Scan shape for the field
	for _, f := range o.shape.fields {
		if (key.isString() && f.keyKind == KeyKindString && f.name == key.name) ||
			(key.isSymbol() && f.keyKind == KeyKindSymbol && f.symbolVal.obj == key.symbolVal.obj) {
			if f.offset < len(o.properties) {
				return o.properties[f.offset], true
			}
			return Undefined, true
		}
	}
	return Undefined, false
}

// GetOwnDescriptor returns the value and attribute flags for an own property.
// Returns (value, writable, enumerable, configurable, exists).
func (o *PlainObject) GetOwnDescriptor(name string) (Value, bool, bool, bool, bool) {
	return o.GetOwnDescriptorByKey(keyFromString(name))
}

// GetOwnDescriptorByKey returns descriptor flags for an own property keyed by PropertyKey.
func (o *PlainObject) GetOwnDescriptorByKey(key PropertyKey) (Value, bool, bool, bool, bool) {
	for _, f := range o.shape.fields {
		if (key.isString() && f.keyKind == KeyKindString && f.name == key.name) ||
			(key.isSymbol() && f.keyKind == KeyKindSymbol && f.symbolVal.obj == key.symbolVal.obj) {
			if f.isAccessor {
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
	return o.GetOwnAccessorByKey(keyFromString(name))
}

// GetOwnAccessorByKey returns accessor pair for an own property by key.
func (o *PlainObject) GetOwnAccessorByKey(key PropertyKey) (Value, Value, bool, bool, bool) {
	for _, f := range o.shape.fields {
		if ((key.isString() && f.keyKind == KeyKindString && f.name == key.name) ||
			(key.isSymbol() && f.keyKind == KeyKindSymbol && f.symbolVal.obj == key.symbolVal.obj)) && f.isAccessor {
			var g, s Value = Undefined, Undefined
			if o.getters != nil {
				if v, ok := o.getters[key.hash()]; ok {
					g = v
				}
			}
			if o.setters != nil {
				if v, ok := o.setters[key.hash()]; ok {
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
	return o.DeleteOwnByKey(keyFromString(name))
}

// DeleteOwnByKey removes an own property by key if present and configurable.
func (o *PlainObject) DeleteOwnByKey(key PropertyKey) bool {
	// Find field index
	idx := -1
	var f Field
	for i := range o.shape.fields {
		if (key.isString() && o.shape.fields[i].keyKind == KeyKindString && o.shape.fields[i].name == key.name) ||
			(key.isSymbol() && o.shape.fields[i].keyKind == KeyKindSymbol && o.shape.fields[i].symbolVal.obj == key.symbolVal.obj) {
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
	// Create new shape without transitions for simplicity and bump version
	o.shape = &Shape{parent: o.shape.parent, fields: newFields, transitions: make(map[string]*Shape), version: o.shape.version + 1}
	o.properties = newProps
	return true
}

// IsOwnPropertyNonConfigurable returns (exists, nonConfigurable) for an own property.
// exists is true if the property exists on this object.
// nonConfigurable is true if the property exists and is not configurable.
func (o *PlainObject) IsOwnPropertyNonConfigurable(name string) (exists bool, nonConfigurable bool) {
	for _, f := range o.shape.fields {
		if f.keyKind == KeyKindString && f.name == name {
			return true, !f.configurable
		}
	}
	return false, false
}

// SetOwn sets or defines an own property. Creates a new shape on first definition.
// If the property exists and is non-writable, this is a no-op.
func (o *PlainObject) SetOwn(name string, v Value) {
	// Backward-compat name path
	for _, f := range o.shape.fields {
		if f.keyKind == KeyKindString && f.name == name {
			// existing property: honor writable flag
			if f.writable {
				o.properties[f.offset] = v
			}
			return
		}
	}
	// new property: regular assignment semantics -> writable: true, enumerable: true, configurable: true
	cur := o.shape
	cur.mu.RLock()
	next, ok := cur.transitions[keyFromString(name).hash()]
	cur.mu.RUnlock()
	if !ok {
		off := len(cur.fields)
		fld := Field{offset: off, name: name, keyKind: KeyKindString, writable: true, enumerable: true, configurable: true}
		newFields := make([]Field, len(cur.fields)+1)
		copy(newFields, cur.fields)
		newFields[len(cur.fields)] = fld
		newTrans := make(map[string]*Shape)
		next = &Shape{parent: cur, fields: newFields, transitions: newTrans, version: cur.version + 1}
		cur.mu.Lock()
		if existing, exists := cur.transitions[keyFromString(name).hash()]; exists {
			next = existing
		} else {
			cur.transitions[keyFromString(name).hash()] = next
		}
		cur.mu.Unlock()
	}
	o.shape = next
	o.properties = append(o.properties, v)
}

// SetOwnNonEnumerable sets or defines an own property as non-enumerable (for built-in methods).
// Creates a new shape on first definition with enumerable: false, writable: true, configurable: true.
func (o *PlainObject) SetOwnNonEnumerable(name string, v Value) {
	// Check if property already exists
	for _, f := range o.shape.fields {
		if f.keyKind == KeyKindString && f.name == name {
			// existing property: honor writable flag
			if f.writable {
				o.properties[f.offset] = v
			}
			return
		}
	}
	// new property: built-in assignment semantics -> writable: true, enumerable: false, configurable: true
	cur := o.shape
	cur.mu.RLock()
	hashKey := keyFromString(name).hash() + "_nonenum" // Different hash to avoid collision with enumerable version
	next, ok := cur.transitions[hashKey]
	cur.mu.RUnlock()
	if !ok {
		off := len(cur.fields)
		fld := Field{offset: off, name: name, keyKind: KeyKindString, writable: true, enumerable: false, configurable: true}
		newFields := make([]Field, len(cur.fields)+1)
		copy(newFields, cur.fields)
		newFields[len(cur.fields)] = fld
		newTrans := make(map[string]*Shape)
		next = &Shape{parent: cur, fields: newFields, transitions: newTrans, version: cur.version + 1}
		cur.mu.Lock()
		if existing, exists := cur.transitions[hashKey]; exists {
			next = existing
		} else {
			cur.transitions[hashKey] = next
		}
		cur.mu.Unlock()
	}
	o.shape = next
	o.properties = append(o.properties, v)
}

// DefineOwnProperty defines or updates an own property with explicit attributes.
// For existing properties, unspecified attributes (nil) will keep previous values.
func (o *PlainObject) DefineOwnProperty(name string, value Value, writable *bool, enumerable *bool, configurable *bool) {
	// Update existing
	for i, f := range o.shape.fields {
		if f.keyKind == KeyKindString && f.name == name {
			// Existing property: enforce non-configurable rules
			newF := f
			convertingFromAccessor := false
			if f.isAccessor {
				// Convert accessor to data property: only if configurable
				if !f.configurable {
					return
				}
				newF.isAccessor = false
				newF.writable = false // Default for new data property
				convertingFromAccessor = true
				// Clean up getter/setter maps
				keyHash := keyFromString(name).hash()
				if o.getters != nil {
					delete(o.getters, keyHash)
				}
				if o.setters != nil {
					delete(o.setters, keyHash)
				}
			}
			// If current non-configurable, cannot change configurable or enumerable
			if !f.configurable {
				if configurable != nil && *configurable != f.configurable {
					return
				}
				if enumerable != nil && *enumerable != f.enumerable {
					return
				}
				// Non-configurable, non-writable properties cannot have writable changed to true
				if f.writable == false && writable != nil && *writable == true {
					return
				}
			}
			// Update value: if configurable, always allow; otherwise only if writable
			if f.configurable || convertingFromAccessor || f.writable {
				o.properties[f.offset] = value
			}
			if writable != nil {
				newF.writable = *writable
			}
			if enumerable != nil {
				newF.enumerable = *enumerable
			}
			if configurable != nil {
				newF.configurable = *configurable
			}
			o.shape.fields[i] = newF
			o.shape.version++
			return
		}
	}
	// New property via descriptor: defaults false unless specified
	cur := o.shape
	off := len(cur.fields)
	fld := Field{offset: off, name: name, keyKind: KeyKindString, writable: false, enumerable: false, configurable: false, isAccessor: false}
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
	next := &Shape{parent: cur, fields: newFields, transitions: make(map[string]*Shape), version: cur.version + 1}
	o.shape = next
	o.properties = append(o.properties, value)
}

// DefineAccessorProperty defines or updates an accessor own property.
func (o *PlainObject) DefineAccessorProperty(name string, getter Value, hasGetter bool, setter Value, hasSetter bool, enumerable *bool, configurable *bool) {
	// Wrapper using string name
	// Find existing field
	for i, f := range o.shape.fields {
		if f.keyKind == KeyKindString && f.name == name {
			// If existing field is not configurable, cannot change it to accessor or modify flags
			if !f.configurable {
				return
			}
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
			o.shape.version++
			if o.getters == nil {
				o.getters = make(map[string]Value)
			}
			if o.setters == nil {
				o.setters = make(map[string]Value)
			}
			if hasGetter {
				o.getters[keyFromString(name).hash()] = getter
			}
			if hasSetter {
				o.setters[keyFromString(name).hash()] = setter
			}
			return
		}
	}
	// New field - for accessors, always create a new shape (don't use transitions)
	// because accessor properties have different semantics than data properties
	cur := o.shape
	off := len(cur.fields)
	fld := Field{offset: off, name: name, keyKind: KeyKindString, writable: false, enumerable: false, configurable: false, isAccessor: true}
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
	next := &Shape{parent: cur, fields: newFields, transitions: newTrans, version: cur.version + 1}
	o.shape = next
	// Ensure maps
	if o.getters == nil {
		o.getters = make(map[string]Value)
	}
	if o.setters == nil {
		o.setters = make(map[string]Value)
	}
	if hasGetter {
		o.getters[keyFromString(name).hash()] = getter
	}
	if hasSetter {
		o.setters[keyFromString(name).hash()] = setter
	}
	// Keep properties slice length consistent
	o.properties = append(o.properties, Undefined)
}

// DefineOwnPropertyByKey defines or updates an own property for arbitrary key kinds.
func (o *PlainObject) DefineOwnPropertyByKey(key PropertyKey, value Value, writable *bool, enumerable *bool, configurable *bool) {
	for i, f := range o.shape.fields {
		match := (key.isString() && f.keyKind == KeyKindString && f.name == key.name) || (key.isSymbol() && f.keyKind == KeyKindSymbol && f.symbolVal.obj == key.symbolVal.obj)
		if match {
			newF := f
			if f.isAccessor {
				// Only allow conversion if configurable
				if !f.configurable {
					return
				}
				newF.isAccessor = false
				newF.writable = false
			}
			if !f.configurable {
				if configurable != nil && *configurable != f.configurable {
					return
				}
				if enumerable != nil && *enumerable != f.enumerable {
					return
				}
			}
			if f.writable == false && writable != nil && *writable == true {
				return
			}
			if f.writable {
				o.properties[f.offset] = value
			}
			if writable != nil {
				newF.writable = *writable
			}
			if enumerable != nil {
				newF.enumerable = *enumerable
			}
			if configurable != nil {
				newF.configurable = *configurable
			}
			o.shape.fields[i] = newF
			o.shape.version++
			return
		}
	}
	// New
	cur := o.shape
	off := len(cur.fields)
	fld := Field{offset: off, name: key.debugName(), keyKind: key.kind, writable: false, enumerable: false, configurable: false, isAccessor: false}
	if key.isSymbol() {
		fld.symbolVal = key.symbolVal
	}
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
	next := &Shape{parent: cur, fields: newFields, transitions: make(map[string]*Shape), version: cur.version + 1}
	o.shape = next
	o.properties = append(o.properties, value)
}

// DefineAccessorPropertyByKey defines or updates an accessor property for arbitrary key kinds.
func (o *PlainObject) DefineAccessorPropertyByKey(key PropertyKey, getter Value, hasGetter bool, setter Value, hasSetter bool, enumerable *bool, configurable *bool) {
	// Find existing field
	for i, f := range o.shape.fields {
		match := (key.isString() && f.keyKind == KeyKindString && f.name == key.name) ||
			(key.isSymbol() && f.keyKind == KeyKindSymbol && f.symbolVal.obj == key.symbolVal.obj)
		if match {
			// If existing field is not configurable, cannot modify
			if !f.configurable {
				return
			}
			newF := f
			newF.isAccessor = true
			if enumerable != nil {
				newF.enumerable = *enumerable
			}
			if configurable != nil {
				newF.configurable = *configurable
			}
			o.shape.fields[i] = newF
			o.shape.version++
			if o.getters == nil {
				o.getters = make(map[string]Value)
			}
			if o.setters == nil {
				o.setters = make(map[string]Value)
			}
			if hasGetter {
				o.getters[key.hash()] = getter
			}
			if hasSetter {
				o.setters[key.hash()] = setter
			}
			return
		}
	}
	// New field
	cur := o.shape
	cur.mu.RLock()
	next, ok := cur.transitions[key.hash()]
	cur.mu.RUnlock()
	if !ok {
		off := len(cur.fields)
		fld := Field{offset: off, name: key.debugName(), keyKind: key.kind, writable: false, enumerable: false, configurable: false, isAccessor: true}
		if key.isSymbol() {
			fld.symbolVal = key.symbolVal
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
		next = &Shape{parent: cur, fields: newFields, transitions: newTrans, version: cur.version + 1}
		cur.mu.Lock()
		if existing, exists := cur.transitions[key.hash()]; exists {
			next = existing
		} else {
			cur.transitions[key.hash()] = next
		}
		cur.mu.Unlock()
	}
	o.shape = next
	if o.getters == nil {
		o.getters = make(map[string]Value)
	}
	if o.setters == nil {
		o.setters = make(map[string]Value)
	}
	if hasGetter {
		o.getters[key.hash()] = getter
	}
	if hasSetter {
		o.setters[key.hash()] = setter
	}
	o.properties = append(o.properties, Undefined)
}

// HasOwn reports whether an own property with the given name exists.
func (o *PlainObject) HasOwn(name string) bool {
	return o.HasOwnByKey(keyFromString(name))
}

func (o *PlainObject) HasOwnByKey(key PropertyKey) bool {
	for _, f := range o.shape.fields {
		if (key.isString() && f.keyKind == KeyKindString && f.name == key.name) ||
			(key.isSymbol() && f.keyKind == KeyKindSymbol && f.symbolVal.obj == key.symbolVal.obj) {
			return true
		}
	}
	return false
}

// DeleteOwn deletes an own property. Not supported for PlainObject; always returns false.
// (removed old stub DeleteOwn; implemented above)

// OwnKeys returns the list of own property names in insertion order.
func (o *PlainObject) OwnKeys() []string {
	// Return only string-named enumerable keys (symbols excluded) in insertion order
	keys := make([]string, 0, len(o.shape.fields))
	for _, f := range o.shape.fields {
		if f.keyKind == KeyKindString && f.enumerable {
			keys = append(keys, f.name)
		}
	}
	return keys
}

// OwnPropertyNames returns the list of all own string property names (including non-enumerable).
// Per ECMAScript spec, integer indices come first (in ascending numeric order), then string keys
// in insertion order.
func (o *PlainObject) OwnPropertyNames() []string {
	// Separate integer indices from string keys
	var integerIndices []int
	var stringKeys []string

	for _, f := range o.shape.fields {
		if f.keyKind == KeyKindString {
			// Check if this is an integer index (non-negative integer string)
			if idx, isInt := tryParseArrayIndex(f.name); isInt {
				integerIndices = append(integerIndices, idx)
			} else {
				stringKeys = append(stringKeys, f.name)
			}
		}
	}

	// Sort integer indices in ascending order
	sortInts(integerIndices)

	// Build result: integer indices first, then string keys in insertion order
	result := make([]string, 0, len(integerIndices)+len(stringKeys))
	for _, idx := range integerIndices {
		result = append(result, intToString(idx))
	}
	result = append(result, stringKeys...)

	return result
}

// tryParseArrayIndex checks if a string represents a valid array index.
// Returns (index, true) if valid, (0, false) otherwise.
// Valid array indices are non-negative integers in range [0, 2^32-1) without leading zeros.
func tryParseArrayIndex(key string) (int, bool) {
	if key == "" {
		return 0, false
	}
	// Leading zeros not allowed (except "0" itself)
	if len(key) > 1 && key[0] == '0' {
		return 0, false
	}
	idx := 0
	for _, ch := range key {
		if ch < '0' || ch > '9' {
			return 0, false
		}
		idx = idx*10 + int(ch-'0')
		// Check for overflow (max array index is 2^32 - 2)
		if idx > 4294967294 {
			return 0, false
		}
	}
	return idx, true
}

// intToString converts an int to string without importing strconv
func intToString(n int) string {
	if n == 0 {
		return "0"
	}
	var digits []byte
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	return string(digits)
}

// sortInts sorts a slice of ints in ascending order (simple insertion sort for small slices)
func sortInts(a []int) {
	for i := 1; i < len(a); i++ {
		for j := i; j > 0 && a[j-1] > a[j]; j-- {
			a[j-1], a[j] = a[j], a[j-1]
		}
	}
}

// OwnSymbolKeys returns the list of own symbol keys in insertion order.
func (o *PlainObject) OwnSymbolKeys() []Value {
	symbols := make([]Value, 0)
	for _, f := range o.shape.fields {
		if f.keyKind == KeyKindSymbol {
			symbols = append(symbols, f.symbolVal)
		}
	}
	return symbols
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
// Returns true if successful, false if the operation failed (e.g. object is non-extensible)
func (o *PlainObject) SetPrototype(proto Value) bool {
	// ES6 9.1.2 [[SetPrototypeOf]]
	current := o.prototype
	if proto.Is(current) {
		return true
	}
	if !o.extensible {
		return false
	}
	o.prototype = proto
	// TODO: Invalidate related caches
	return true
}

// GetPrivateField retrieves a private field value (ECMAScript # fields)
// Returns (value, true) if the field exists, (Undefined, false) otherwise
func (o *PlainObject) GetPrivateField(name string) (Value, bool) {
	if o.privateFields == nil {
		return Undefined, false
	}
	v, ok := o.privateFields[name]
	if !ok {
		return Undefined, false
	}
	return v, true
}

// SetPrivateField sets a private field value (ECMAScript # fields)
// Creates the privateFields map if it doesn't exist
func (o *PlainObject) SetPrivateField(name string, value Value) {
	if o.privateFields == nil {
		o.privateFields = make(map[string]Value)
	}
	o.privateFields[name] = value
}

// HasPrivateField checks if a private field exists
func (o *PlainObject) HasPrivateField(name string) bool {
	if o.privateFields == nil {
		return false
	}
	_, ok := o.privateFields[name]
	return ok
}

// GetPrivateMethod retrieves a private method value (ECMAScript # methods)
func (o *PlainObject) GetPrivateMethod(name string) (Value, bool) {
	if o.privateMethods == nil {
		return Undefined, false
	}
	v, ok := o.privateMethods[name]
	if !ok {
		return Undefined, false
	}
	return v, true
}

// SetPrivateMethod sets a private method value (ECMAScript # methods)
// Methods are not writable - any subsequent assignment throws TypeError
func (o *PlainObject) SetPrivateMethod(name string, value Value) {
	if o.privateMethods == nil {
		o.privateMethods = make(map[string]Value)
	}
	o.privateMethods[name] = value
}

// IsPrivateMethod checks if a private method exists
func (o *PlainObject) IsPrivateMethod(name string) bool {
	if o.privateMethods == nil {
		return false
	}
	_, ok := o.privateMethods[name]
	return ok
}

// SetPrivateAccessor sets a private getter/setter pair (ECMAScript # accessor properties)
// Pass Undefined for getter or setter if not defined
func (o *PlainObject) SetPrivateAccessor(name string, getter Value, setter Value) {
	if !getter.IsUndefined() {
		if o.privateGetters == nil {
			o.privateGetters = make(map[string]Value)
		}
		o.privateGetters[name] = getter
	}
	if !setter.IsUndefined() {
		if o.privateSetters == nil {
			o.privateSetters = make(map[string]Value)
		}
		o.privateSetters[name] = setter
	}
}

// GetPrivateAccessor retrieves a private getter/setter pair
// Returns (getter, setter, exists)
func (o *PlainObject) GetPrivateAccessor(name string) (Value, Value, bool) {
	var hasGetter, hasSetter bool
	var getter, setter Value = Undefined, Undefined

	if o.privateGetters != nil {
		getter, hasGetter = o.privateGetters[name]
	}
	if o.privateSetters != nil {
		setter, hasSetter = o.privateSetters[name]
	}

	if !hasGetter && !hasSetter {
		return Undefined, Undefined, false
	}

	return getter, setter, true
}

// IsPrivateAccessor checks if a private field is an accessor (has getter or setter)
func (o *PlainObject) IsPrivateAccessor(name string) bool {
	if o.privateGetters != nil {
		if _, hasGetter := o.privateGetters[name]; hasGetter {
			return true
		}
	}
	if o.privateSetters != nil {
		if _, hasSetter := o.privateSetters[name]; hasSetter {
			return true
		}
	}
	return false
}

// IsExtensible returns whether new properties can be added to this object
func (o *PlainObject) IsExtensible() bool {
	return o.extensible
}

// SetExtensible sets the extensible flag for this object
// Per ECMAScript spec, once set to false, it cannot be set back to true
func (o *PlainObject) SetExtensible(extensible bool) {
	if !extensible {
		o.extensible = false
	}
	// Silently ignore attempts to set extensible back to true
}

type DictObject struct {
	Object
	prototype  Value
	properties map[string]Value
	// Extensible flag - when false, no new properties can be added
	extensible bool
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

// IsOwnPropertyNonConfigurable returns (exists, nonConfigurable) for an own property.
// DictObject properties are always configurable, so nonConfigurable is always false.
func (d *DictObject) IsOwnPropertyNonConfigurable(name string) (exists bool, nonConfigurable bool) {
	_, exists = d.properties[name]
	return exists, false // DictObject properties are always configurable
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

// OwnPropertyNames returns the sorted list of own property names (alias for OwnKeys as DictObject has no non-enumerable props).
func (d *DictObject) OwnPropertyNames() []string {
	return d.OwnKeys()
}

// IsExtensible returns whether new properties can be added to this object
func (d *DictObject) IsExtensible() bool {
	return d.extensible
}

// SetExtensible sets the extensible flag for this object
func (d *DictObject) SetExtensible(extensible bool) {
	if !extensible {
		d.extensible = false
	}
}

// GetPrototype returns the object's prototype.
func (d *DictObject) GetPrototype() Value {
	return d.prototype
}

// SetPrototype sets the object's prototype.
func (d *DictObject) SetPrototype(proto Value) bool {
	current := d.prototype
	if proto.Is(current) {
		return true
	}
	if !d.extensible {
		return false
	}
	d.prototype = proto
	// TODO: Invalidate related caches
	return true
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

// ClearShapeCache clears the global RootShape transition map to prevent memory bloat
// This should be called periodically in test runners that create many short-lived VM instances
func ClearShapeCache() {
	if RootShape != nil {
		RootShape.mu.Lock()
		// Clear all transitions to allow GC to free the shape tree
		RootShape.transitions = make(map[string]*Shape)
		RootShape.mu.Unlock()
	}
}

func NewObject(proto Value) Value {
	// Create a new PlainObject and set its prototype to the shared DefaultObjectPrototype
	prototype := DefaultObjectPrototype
	if proto.IsObject() {
		prototype = proto
	}
	plainObj := &PlainObject{prototype: prototype, shape: RootShape, extensible: true}
	return Value{typ: TypeObject, obj: unsafe.Pointer(plainObj)}
}

func NewDictObject(proto Value) Value {
	prototype := DefaultObjectPrototype
	if proto.IsObject() {
		prototype = proto
	}
	dictObj := &DictObject{prototype: prototype, properties: make(map[string]Value), extensible: true}
	return Value{typ: TypeDictObject, obj: unsafe.Pointer(dictObj)}
}
