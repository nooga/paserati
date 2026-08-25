package vm

import (
	"encoding/binary"
	"math"
	"math/big"
	"unsafe"
)

// TypedArrayKind represents the different typed array types
type TypedArrayKind uint8

const (
	TypedArrayInt8 TypedArrayKind = iota
	TypedArrayUint8
	TypedArrayUint8Clamped
	TypedArrayInt16
	TypedArrayUint16
	TypedArrayInt32
	TypedArrayUint32
	TypedArrayFloat32
	TypedArrayFloat64
	TypedArrayBigInt64
	TypedArrayBigUint64
)

// ArrayBufferObject represents a raw binary data buffer
type ArrayBufferObject struct {
	Object
	data       []byte
	detached   bool
	properties map[string]Value // Own properties (e.g., constructor override)
	prototype  Value            // Per-instance [[Prototype]] override for subclassing; Undefined = intrinsic
}

func (ab *ArrayBufferObject) GetPrototype() Value  { return ab.prototype }
func (ab *ArrayBufferObject) SetPrototype(p Value) { ab.prototype = p }

// GetData returns the underlying byte slice
func (ab *ArrayBufferObject) GetData() []byte {
	return ab.data
}

// IsDetached returns whether the buffer has been detached
func (ab *ArrayBufferObject) IsDetached() bool {
	return ab.detached
}

// Detach detaches the ArrayBuffer, making it unusable
func (ab *ArrayBufferObject) Detach() {
	ab.detached = true
	ab.data = nil
}

// GetOwnProperty returns an own property value
func (ab *ArrayBufferObject) GetOwnProperty(name string) (Value, bool) {
	if ab.properties == nil {
		return Undefined, false
	}
	v, ok := ab.properties[name]
	return v, ok
}

// SetOwnProperty sets an own property value
func (ab *ArrayBufferObject) SetOwnProperty(name string, value Value) {
	if ab.properties == nil {
		ab.properties = make(map[string]Value)
	}
	ab.properties[name] = value
}

// HasOwnProperty checks if the buffer has an own property
func (ab *ArrayBufferObject) HasOwnProperty(name string) bool {
	if ab.properties == nil {
		return false
	}
	_, ok := ab.properties[name]
	return ok
}

// BufferData is an interface for ArrayBuffer-like objects
// Both ArrayBuffer and SharedArrayBuffer implement this interface
type BufferData interface {
	GetData() []byte
	IsDetached() bool
}

// SharedArrayBufferObject represents a shared binary data buffer
// Unlike ArrayBuffer, SharedArrayBuffer cannot be detached and is designed
// for shared memory between workers (though in this implementation, we don't
// have multi-threading support yet)
type SharedArrayBufferObject struct {
	Object
	data       []byte
	properties map[string]Value // Own properties (e.g., constructor override)
	prototype  Value            // Per-instance [[Prototype]] override for subclassing; Undefined = intrinsic
}

func (sab *SharedArrayBufferObject) GetPrototype() Value  { return sab.prototype }
func (sab *SharedArrayBufferObject) SetPrototype(p Value) { sab.prototype = p }

// IsDetached always returns false for SharedArrayBuffer (cannot be detached)
func (sab *SharedArrayBufferObject) IsDetached() bool {
	return false
}

// GetData returns the underlying byte slice
func (sab *SharedArrayBufferObject) GetData() []byte {
	return sab.data
}

// ByteLength returns the length in bytes
func (sab *SharedArrayBufferObject) ByteLength() int {
	return len(sab.data)
}

// GetOwnProperty returns an own property value
func (sab *SharedArrayBufferObject) GetOwnProperty(name string) (Value, bool) {
	if sab.properties == nil {
		return Undefined, false
	}
	v, ok := sab.properties[name]
	return v, ok
}

// SetOwnProperty sets an own property value
func (sab *SharedArrayBufferObject) SetOwnProperty(name string, value Value) {
	if sab.properties == nil {
		sab.properties = make(map[string]Value)
	}
	sab.properties[name] = value
}

// HasOwnProperty checks if the buffer has an own property
func (sab *SharedArrayBufferObject) HasOwnProperty(name string) bool {
	if sab.properties == nil {
		return false
	}
	_, ok := sab.properties[name]
	return ok
}

// TypedArrayObject represents a typed view into an ArrayBuffer or SharedArrayBuffer
type TypedArrayObject struct {
	Object
	buffer      BufferData
	byteOffset  int
	byteLength  int
	length      int // number of elements
	elementType TypedArrayKind
	properties  map[string]Value // Own properties (e.g., constructor override)
	prototype   Value            // Per-instance [[Prototype]] override for subclassing; Undefined = intrinsic
}

func (ta *TypedArrayObject) GetPrototype() Value  { return ta.prototype }
func (ta *TypedArrayObject) SetPrototype(p Value) { ta.prototype = p }

// GetOwnProperty returns an own property value (non-index properties)
func (ta *TypedArrayObject) GetOwnProperty(name string) (Value, bool) {
	if ta.properties == nil {
		return Undefined, false
	}
	v, ok := ta.properties[name]
	return v, ok
}

// SetOwnProperty sets an own property value (non-index properties)
func (ta *TypedArrayObject) SetOwnProperty(name string, value Value) {
	if ta.properties == nil {
		ta.properties = make(map[string]Value)
	}
	ta.properties[name] = value
}

// HasOwnProperty checks if the TypedArray has an own property
func (ta *TypedArrayObject) HasOwnProperty(name string) bool {
	if ta.properties == nil {
		return false
	}
	_, ok := ta.properties[name]
	return ok
}

// Getter methods for TypedArrayObject

// GetBuffer returns the underlying buffer as an ArrayBufferObject (for backwards compatibility)
// Returns nil if the buffer is a SharedArrayBuffer
func (ta *TypedArrayObject) GetBuffer() *ArrayBufferObject {
	if ab, ok := ta.buffer.(*ArrayBufferObject); ok {
		return ab
	}
	return nil
}

// GetBufferData returns the underlying buffer (ArrayBuffer or SharedArrayBuffer)
func (ta *TypedArrayObject) GetBufferData() BufferData {
	return ta.buffer
}

// IsSharedBuffer returns true if the underlying buffer is a SharedArrayBuffer
func (ta *TypedArrayObject) IsSharedBuffer() bool {
	_, ok := ta.buffer.(*SharedArrayBufferObject)
	return ok
}

// GetSharedBuffer returns the underlying SharedArrayBuffer, or nil if not shared
func (ta *TypedArrayObject) GetSharedBuffer() *SharedArrayBufferObject {
	if sab, ok := ta.buffer.(*SharedArrayBufferObject); ok {
		return sab
	}
	return nil
}

func (ta *TypedArrayObject) GetByteOffset() int {
	return ta.byteOffset
}

func (ta *TypedArrayObject) GetByteLength() int {
	return ta.byteLength
}

func (ta *TypedArrayObject) GetLength() int {
	return ta.length
}

func (ta *TypedArrayObject) GetBytesPerElement() int {
	return ta.elementType.BytesPerElement()
}

func (ta *TypedArrayObject) GetElementType() TypedArrayKind {
	return ta.elementType
}

// IsOutOfBounds reports whether ta's view is no longer valid over its
// backing buffer. Today that's exactly detachment - checked through the
// BufferData interface so it's correct for both ArrayBuffer and
// SharedArrayBuffer (unlike GetBuffer().IsDetached(), which nil-derefs for a
// SharedArrayBuffer-backed view since GetBuffer only returns *ArrayBufferObject).
// Extend this once resizable ArrayBuffers exist: a fixed-length view whose
// buffer has shrunk below [[ByteOffset]]+[[ByteLength]] is out of bounds too,
// per the spec's IsTypedArrayOutOfBounds.
func (ta *TypedArrayObject) IsOutOfBounds() bool {
	return ta.buffer.IsDetached()
}

// Helper to get bytes per element for each typed array kind
func (kind TypedArrayKind) BytesPerElement() int {
	switch kind {
	case TypedArrayInt8, TypedArrayUint8, TypedArrayUint8Clamped:
		return 1
	case TypedArrayInt16, TypedArrayUint16:
		return 2
	case TypedArrayInt32, TypedArrayUint32, TypedArrayFloat32:
		return 4
	case TypedArrayFloat64, TypedArrayBigInt64, TypedArrayBigUint64:
		return 8
	default:
		return 0
	}
}

// Name returns the ECMAScript constructor name for this TypedArray kind
func (kind TypedArrayKind) Name() string {
	switch kind {
	case TypedArrayInt8:
		return "Int8Array"
	case TypedArrayUint8:
		return "Uint8Array"
	case TypedArrayUint8Clamped:
		return "Uint8ClampedArray"
	case TypedArrayInt16:
		return "Int16Array"
	case TypedArrayUint16:
		return "Uint16Array"
	case TypedArrayInt32:
		return "Int32Array"
	case TypedArrayUint32:
		return "Uint32Array"
	case TypedArrayFloat32:
		return "Float32Array"
	case TypedArrayFloat64:
		return "Float64Array"
	case TypedArrayBigInt64:
		return "BigInt64Array"
	case TypedArrayBigUint64:
		return "BigUint64Array"
	default:
		return "TypedArray"
	}
}

// GetTypedArrayElement gets an element at the given index
func (ta *TypedArrayObject) GetElement(index int) Value {
	if index < 0 || index >= ta.length {
		return Undefined
	}

	// Check if buffer is detached
	if ta.buffer.IsDetached() {
		return Undefined // In strict mode this should throw, but returning undefined for now
	}

	offset := ta.byteOffset + index*ta.elementType.BytesPerElement()
	data := ta.buffer.GetData()[offset:]

	switch ta.elementType {
	case TypedArrayInt8:
		return Number(float64(int8(data[0])))
	case TypedArrayUint8:
		return Number(float64(data[0]))
	case TypedArrayUint8Clamped:
		return Number(float64(data[0]))
	case TypedArrayInt16:
		return Number(float64(int16(binary.LittleEndian.Uint16(data))))
	case TypedArrayUint16:
		return Number(float64(binary.LittleEndian.Uint16(data)))
	case TypedArrayInt32:
		return Number(float64(int32(binary.LittleEndian.Uint32(data))))
	case TypedArrayUint32:
		return Number(float64(binary.LittleEndian.Uint32(data)))
	case TypedArrayFloat32:
		bits := binary.LittleEndian.Uint32(data)
		return Number(float64(math.Float32frombits(bits)))
	case TypedArrayFloat64:
		bits := binary.LittleEndian.Uint64(data)
		return Number(math.Float64frombits(bits))
	case TypedArrayBigInt64:
		i64 := int64(binary.LittleEndian.Uint64(data))
		return NewBigInt(big.NewInt(i64))
	case TypedArrayBigUint64:
		u64 := binary.LittleEndian.Uint64(data)
		bi := new(big.Int).SetUint64(u64)
		return NewBigInt(bi)
	default:
		return Undefined
	}
}

// twoTo64 is 2^64, used to compute BigInt wraparound the same way the spec's
// ToBigInt64/ToBigUint64 abstract operations do (arbitrary-precision modulo,
// not a range-limited Int64()/Uint64() call).
var twoTo64 = new(big.Int).Lsh(big.NewInt(1), 64)

// BigToUint64Wrapped reduces an arbitrary-precision BigInt modulo 2^64,
// matching ToBigUint64. big.Int.Uint64() is not suitable here: for a negative
// value it returns the magnitude's low bits with the sign discarded (e.g.
// -5 -> 5) rather than the two's-complement wraparound (2^64-5) the spec
// requires. Exported so callers writing raw BigInt64/BigUint64 array bytes
// outside this package (e.g. pkg/builtins/atomics_init.go, pkg/vm/dataview.go)
// share the same conversion instead of re-deriving it.
func BigToUint64Wrapped(bi *big.Int) uint64 {
	m := new(big.Int).Mod(bi, twoTo64) // Mod result is always in [0, 2^64) for a positive modulus
	return m.Uint64()
}

// BigToInt64Wrapped reduces an arbitrary-precision BigInt modulo 2^64 and
// reinterprets the top bit as sign, matching ToBigInt64.
func BigToInt64Wrapped(bi *big.Int) int64 {
	return int64(BigToUint64Wrapped(bi))
}

// JSWrapInt performs ECMAScript-style modular wraparound of a float64 into an
// unsigned integer of the given bit width (8/16/32), matching the ToInt8/
// ToUint8/ToInt16/ToUint16/ToInt32/ToUint32 abstract operations. A direct Go
// cast like int16(num) is only well-defined when num already fits; for
// out-of-range floats (e.g. writing 0xF7F7F7F7 into a Uint16Array element)
// float-to-int conversion in Go is implementation-specific, so this computes
// the wraparound explicitly via math.Mod instead of relying on the cast.
// Exported so other packages writing raw typed-array bytes (e.g.
// pkg/builtins/atomics_init.go's atomicStore) share this instead of the same
// unsafe cast this function exists to replace.
func JSWrapInt(num float64, bits uint) uint64 {
	if math.IsNaN(num) || math.IsInf(num, 0) {
		return 0
	}
	mod := math.Mod(math.Trunc(num), math.Pow(2, float64(bits)))
	if mod < 0 {
		mod += math.Pow(2, float64(bits))
	}
	return uint64(mod)
}

// SetTypedArrayElement sets an element at the given index
func (ta *TypedArrayObject) SetElement(index int, value Value) {
	if index < 0 || index >= ta.length {
		return
	}

	// Check if buffer is detached
	if ta.buffer.IsDetached() {
		return // In strict mode this should throw, but silently failing for now
	}

	// Convert value to number
	num := value.ToFloat()
	offset := ta.byteOffset + index*ta.elementType.BytesPerElement()
	data := ta.buffer.GetData()[offset:]

	switch ta.elementType {
	case TypedArrayInt8, TypedArrayUint8:
		data[0] = byte(JSWrapInt(num, 8))
	case TypedArrayUint8Clamped:
		// Clamp between 0 and 255
		if num < 0 {
			data[0] = 0
		} else if num > 255 {
			data[0] = 255
		} else {
			data[0] = byte(num)
		}
	case TypedArrayInt16, TypedArrayUint16:
		binary.LittleEndian.PutUint16(data, uint16(JSWrapInt(num, 16)))
	case TypedArrayInt32, TypedArrayUint32:
		binary.LittleEndian.PutUint32(data, uint32(JSWrapInt(num, 32)))
	case TypedArrayFloat32:
		binary.LittleEndian.PutUint32(data, math.Float32bits(float32(num)))
	case TypedArrayFloat64:
		binary.LittleEndian.PutUint64(data, math.Float64bits(num))
	case TypedArrayBigInt64:
		// For BigInt64Array, value should be a BigInt
		if value.IsBigInt() {
			binary.LittleEndian.PutUint64(data, uint64(BigToInt64Wrapped(value.AsBigInt())))
		} else {
			// Convert number to int64
			binary.LittleEndian.PutUint64(data, uint64(int64(num)))
		}
	case TypedArrayBigUint64:
		// For BigUint64Array, value should be a BigInt
		if value.IsBigInt() {
			binary.LittleEndian.PutUint64(data, BigToUint64Wrapped(value.AsBigInt()))
		} else {
			// Convert number to uint64
			binary.LittleEndian.PutUint64(data, uint64(num))
		}
	}
}

// Value type helpers

func NewArrayBuffer(size int) Value {
	if size < 0 {
		return Undefined // Should be an error
	}
	buffer := &ArrayBufferObject{
		data: make([]byte, size),
	}
	return Value{typ: TypeArrayBuffer, obj: unsafe.Pointer(buffer)}
}

// NewArrayBufferFromObject creates a Value from an existing ArrayBufferObject
func NewArrayBufferFromObject(buffer *ArrayBufferObject) Value {
	if buffer == nil {
		return Undefined
	}
	return Value{typ: TypeArrayBuffer, obj: unsafe.Pointer(buffer)}
}

// NewSharedArrayBuffer creates a new SharedArrayBuffer with the given size
func NewSharedArrayBuffer(size int) Value {
	if size < 0 {
		return Undefined // Should be an error
	}
	buffer := &SharedArrayBufferObject{
		data: make([]byte, size),
	}
	return Value{typ: TypeSharedArrayBuffer, obj: unsafe.Pointer(buffer)}
}

// NewSharedArrayBufferFromObject creates a Value from an existing SharedArrayBufferObject
func NewSharedArrayBufferFromObject(buffer *SharedArrayBufferObject) Value {
	if buffer == nil {
		return Undefined
	}
	return Value{typ: TypeSharedArrayBuffer, obj: unsafe.Pointer(buffer)}
}

func NewTypedArray(kind TypedArrayKind, lengthOrBuffer interface{}, byteOffset, length int) Value {
	var buffer BufferData
	var arrayLength int
	var arrayByteOffset int

	switch arg := lengthOrBuffer.(type) {
	case int:
		// Creating with just a length
		arrayLength = arg
		bytesNeeded := arrayLength * kind.BytesPerElement()
		buffer = &ArrayBufferObject{data: make([]byte, bytesNeeded)}
		arrayByteOffset = 0
	case *ArrayBufferObject:
		// Creating from existing ArrayBuffer
		buffer = arg
		arrayByteOffset = byteOffset
		if length > 0 {
			arrayLength = length
		} else {
			// Calculate length from buffer size
			remainingBytes := len(buffer.GetData()) - arrayByteOffset
			arrayLength = remainingBytes / kind.BytesPerElement()
		}
	case *SharedArrayBufferObject:
		// Creating from existing SharedArrayBuffer
		buffer = arg
		arrayByteOffset = byteOffset
		if length > 0 {
			arrayLength = length
		} else {
			// Calculate length from buffer size
			remainingBytes := len(buffer.GetData()) - arrayByteOffset
			arrayLength = remainingBytes / kind.BytesPerElement()
		}
	case []Value:
		// Creating from array of values
		arrayLength = len(arg)
		bytesNeeded := arrayLength * kind.BytesPerElement()
		newBuffer := &ArrayBufferObject{data: make([]byte, bytesNeeded)}

		// Initialize with values
		ta := &TypedArrayObject{
			buffer:      newBuffer,
			byteOffset:  0,
			byteLength:  bytesNeeded,
			length:      arrayLength,
			elementType: kind,
		}
		for i, v := range arg {
			ta.SetElement(i, v)
		}
		return Value{typ: TypeTypedArray, obj: unsafe.Pointer(ta)}
	default:
		return Undefined
	}

	ta := &TypedArrayObject{
		buffer:      buffer,
		byteOffset:  arrayByteOffset,
		byteLength:  arrayLength * kind.BytesPerElement(),
		length:      arrayLength,
		elementType: kind,
	}

	return Value{typ: TypeTypedArray, obj: unsafe.Pointer(ta)}
}

// Value type accessors

func (v Value) AsArrayBuffer() *ArrayBufferObject {
	if v.typ == TypeArrayBuffer {
		return (*ArrayBufferObject)(v.obj)
	}
	return nil
}

func (v Value) AsSharedArrayBuffer() *SharedArrayBufferObject {
	if v.typ == TypeSharedArrayBuffer {
		return (*SharedArrayBufferObject)(v.obj)
	}
	return nil
}

func (v Value) AsTypedArray() *TypedArrayObject {
	if v.typ == TypeTypedArray {
		return (*TypedArrayObject)(v.obj)
	}
	return nil
}
