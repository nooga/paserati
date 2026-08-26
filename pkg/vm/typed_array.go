package vm

import (
	"encoding/binary"
	"errors"
	"math"
	"math/big"
	"unsafe"
)

// Sentinel errors for resizable/growable/transferable ArrayBuffer operations.
// Callers (pkg/builtins) map these to the appropriate TypeError/RangeError.
var (
	errArrayBufferNotResizable      = errors.New("ArrayBuffer is not resizable")
	errArrayBufferDetached          = errors.New("Cannot perform operation on a detached ArrayBuffer")
	errArrayBufferInvalidLength     = errors.New("Invalid array buffer length")
	errSharedArrayBufferNotGrowable = errors.New("SharedArrayBuffer is not growable")
	errSharedArrayBufferShrink      = errors.New("SharedArrayBuffer.prototype.grow: new length must not be smaller than the current length")
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
	data          []byte
	detached      bool
	maxByteLength int              // -1 if the buffer is not resizable, else the max byteLength given at construction
	properties    map[string]Value // Own properties (e.g., constructor override)
	prototype     Value            // Per-instance [[Prototype]] override for subclassing; Undefined = intrinsic
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

// IsResizable reports whether this ArrayBuffer was constructed with a
// maxByteLength option (ES2024 resizable ArrayBuffer).
func (ab *ArrayBufferObject) IsResizable() bool {
	return ab.maxByteLength >= 0
}

// MaxByteLength returns the buffer's [[ArrayBufferMaxByteLength]], or -1 if
// the buffer is not resizable.
func (ab *ArrayBufferObject) MaxByteLength() int {
	return ab.maxByteLength
}

// Resize implements ArrayBuffer.prototype.resize's [[ArrayBufferByteLength]]
// update: it must be resizable, not detached, and newLen must be within
// [0, maxByteLength]. Bytes exposed by growth are zero-filled; bytes beyond
// a shrink are dropped (and zero-filled again if later re-exposed by growth).
func (ab *ArrayBufferObject) Resize(newLen int) error {
	if !ab.IsResizable() {
		return errArrayBufferNotResizable
	}
	if ab.detached {
		return errArrayBufferDetached
	}
	if newLen < 0 || newLen > ab.maxByteLength {
		return errArrayBufferInvalidLength
	}
	oldLen := len(ab.data)
	if newLen <= cap(ab.data) {
		ab.data = ab.data[:newLen]
		for i := oldLen; i < newLen; i++ {
			ab.data[i] = 0
		}
	} else {
		newData := make([]byte, newLen)
		copy(newData, ab.data)
		ab.data = newData
	}
	return nil
}

// Transfer implements ArrayBufferCopyAndDetach: it detaches ab and returns a
// new ArrayBufferObject holding ab's data, resized to newLen (zero-padded if
// growing, truncated if shrinking). newLen < 0 means "use ab's current
// byteLength". When preserveResizability is true and ab is itself resizable,
// the new buffer keeps ab's maxByteLength (and newLen must not exceed it);
// otherwise the new buffer is fixed-length.
func (ab *ArrayBufferObject) Transfer(newLen int, preserveResizability bool) (*ArrayBufferObject, error) {
	if ab.detached {
		return nil, errArrayBufferDetached
	}
	if newLen < 0 {
		newLen = len(ab.data)
	}
	newMax := -1
	if preserveResizability && ab.IsResizable() {
		newMax = ab.maxByteLength
		if newLen > newMax {
			return nil, errArrayBufferInvalidLength
		}
	}

	var newData []byte
	if newMax >= 0 {
		newData = make([]byte, newLen, newMax)
	} else {
		newData = make([]byte, newLen)
	}
	copy(newData, ab.data)

	newBuffer := &ArrayBufferObject{data: newData, maxByteLength: newMax}

	// Detach the source per spec, regardless of destination resizability.
	ab.detached = true
	ab.data = nil

	return newBuffer, nil
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
	data          []byte
	maxByteLength int              // -1 if not growable, else [[ArrayBufferMaxByteLength]]
	properties    map[string]Value // Own properties (e.g., constructor override)
	prototype     Value            // Per-instance [[Prototype]] override for subclassing; Undefined = intrinsic
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

// IsGrowable reports whether this SharedArrayBuffer was constructed with a
// maxByteLength option (ES2024 growable SharedArrayBuffer).
func (sab *SharedArrayBufferObject) IsGrowable() bool {
	return sab.maxByteLength >= 0
}

// MaxByteLength returns the buffer's [[ArrayBufferMaxByteLength]], or -1 if
// the buffer is not growable.
func (sab *SharedArrayBufferObject) MaxByteLength() int {
	return sab.maxByteLength
}

// Grow implements SharedArrayBuffer.prototype.grow: it must be growable and
// newLen must be within [current length, maxByteLength]. Growable SABs
// preallocate capacity up to maxByteLength at construction time, so growth
// never reallocates (matching the spec's requirement that growth be
// observable in-place by other views/agents).
func (sab *SharedArrayBufferObject) Grow(newLen int) error {
	if !sab.IsGrowable() {
		return errSharedArrayBufferNotGrowable
	}
	if newLen > sab.maxByteLength {
		return errArrayBufferInvalidLength
	}
	oldLen := len(sab.data)
	if newLen < oldLen {
		return errSharedArrayBufferShrink
	}
	if newLen <= cap(sab.data) {
		sab.data = sab.data[:newLen]
	} else {
		newData := make([]byte, newLen)
		copy(newData, sab.data)
		sab.data = newData
	}
	for i := oldLen; i < newLen; i++ {
		sab.data[i] = 0
	}
	return nil
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
	byteLength  int  // fixed byte length; ignored (recomputed live) when trackLength is true
	length      int  // fixed number of elements; ignored (recomputed live) when trackLength is true
	trackLength bool // auto length-tracking view: constructed over a resizable/growable buffer with no explicit length, so length/byteLength follow the buffer's live size
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

// GetByteLength returns the view's current byte length: 0 if out of bounds,
// live-recomputed from the buffer's current size for a length-tracking view,
// or the fixed [[ByteLength]] otherwise.
func (ta *TypedArrayObject) GetByteLength() int {
	if ta.IsOutOfBounds() {
		return 0
	}
	if ta.trackLength {
		return ta.trackingLength() * ta.elementType.BytesPerElement()
	}
	return ta.byteLength
}

// GetLength returns the view's current element count: 0 if out of bounds,
// live-recomputed from the buffer's current size for a length-tracking view,
// or the fixed element count otherwise.
func (ta *TypedArrayObject) GetLength() int {
	if ta.IsOutOfBounds() {
		return 0
	}
	if ta.trackLength {
		return ta.trackingLength()
	}
	return ta.length
}

// trackingLength computes the live element count of a length-tracking view
// from the buffer's current byte length. Callers must ensure the view isn't
// out of bounds first.
func (ta *TypedArrayObject) trackingLength() int {
	bufLen := len(ta.buffer.GetData())
	if ta.byteOffset > bufLen {
		return 0
	}
	return (bufLen - ta.byteOffset) / ta.elementType.BytesPerElement()
}

// IsLengthTracking reports whether this view auto-tracks its buffer's live
// byteLength (constructed over a resizable/growable buffer with no explicit
// length argument).
func (ta *TypedArrayObject) IsLengthTracking() bool {
	return ta.trackLength
}

func (ta *TypedArrayObject) GetBytesPerElement() int {
	return ta.elementType.BytesPerElement()
}

func (ta *TypedArrayObject) GetElementType() TypedArrayKind {
	return ta.elementType
}

// IsOutOfBounds reports whether ta's view is no longer valid over its
// backing buffer: detached outright, a length-tracking view whose byteOffset
// now exceeds the (possibly shrunk) buffer, or a fixed-length view whose
// [[ByteOffset]]+[[ByteLength]] now exceeds the buffer's current size -
// per the spec's IsTypedArrayOutOfBounds.
func (ta *TypedArrayObject) IsOutOfBounds() bool {
	if ta.buffer.IsDetached() {
		return true
	}
	bufLen := len(ta.buffer.GetData())
	if ta.trackLength {
		return ta.byteOffset > bufLen
	}
	return ta.byteOffset+ta.byteLength > bufLen
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
	// GetLength() returns 0 when out of bounds (detached, or shrunk past this
	// view's range), so the bounds check below also covers that case.
	if index < 0 || index >= ta.GetLength() {
		return Undefined
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
	// GetLength() returns 0 when out of bounds (detached, or shrunk past this
	// view's range), so the bounds check below also covers that case.
	if index < 0 || index >= ta.GetLength() {
		return
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
		data:          make([]byte, size),
		maxByteLength: -1,
	}
	return Value{typ: TypeArrayBuffer, obj: unsafe.Pointer(buffer)}
}

// NewResizableArrayBuffer creates a new resizable ArrayBuffer (ES2024) with
// the given initial byteLength and maxByteLength. Capacity is preallocated
// up to maxByteLength so later Resize calls within that cap don't reallocate.
func NewResizableArrayBuffer(size, maxByteLength int) Value {
	if size < 0 || maxByteLength < size {
		return Undefined // Should be an error
	}
	buffer := &ArrayBufferObject{
		data:          make([]byte, size, maxByteLength),
		maxByteLength: maxByteLength,
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
		data:          make([]byte, size),
		maxByteLength: -1,
	}
	return Value{typ: TypeSharedArrayBuffer, obj: unsafe.Pointer(buffer)}
}

// NewGrowableSharedArrayBuffer creates a new growable SharedArrayBuffer
// (ES2024) with the given initial byteLength and maxByteLength. Capacity is
// preallocated up to maxByteLength so Grow never reallocates in place.
func NewGrowableSharedArrayBuffer(size, maxByteLength int) Value {
	if size < 0 || maxByteLength < size {
		return Undefined // Should be an error
	}
	buffer := &SharedArrayBufferObject{
		data:          make([]byte, size, maxByteLength),
		maxByteLength: maxByteLength,
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

// NewTypedArray creates a TypedArray view. length < 0 means "not specified"
// (auto-computed from the buffer's remaining bytes, and length-tracking if
// the buffer is resizable/growable); length >= 0, including 0, is an
// explicit fixed length.
func NewTypedArray(kind TypedArrayKind, lengthOrBuffer interface{}, byteOffset, length int) Value {
	var buffer BufferData
	var arrayLength int
	var arrayByteOffset int
	var trackLength bool

	switch arg := lengthOrBuffer.(type) {
	case int:
		// Creating with just a length
		arrayLength = arg
		bytesNeeded := arrayLength * kind.BytesPerElement()
		buffer = &ArrayBufferObject{data: make([]byte, bytesNeeded), maxByteLength: -1}
		arrayByteOffset = 0
	case *ArrayBufferObject:
		// Creating from existing ArrayBuffer
		buffer = arg
		arrayByteOffset = byteOffset
		if length >= 0 {
			arrayLength = length
		} else if arg.IsResizable() {
			// No explicit length over a resizable buffer: auto length-track.
			trackLength = true
		} else {
			// Calculate length from buffer size
			remainingBytes := len(buffer.GetData()) - arrayByteOffset
			arrayLength = remainingBytes / kind.BytesPerElement()
		}
	case *SharedArrayBufferObject:
		// Creating from existing SharedArrayBuffer
		buffer = arg
		arrayByteOffset = byteOffset
		if length >= 0 {
			arrayLength = length
		} else if arg.IsGrowable() {
			// No explicit length over a growable buffer: auto length-track.
			trackLength = true
		} else {
			// Calculate length from buffer size
			remainingBytes := len(buffer.GetData()) - arrayByteOffset
			arrayLength = remainingBytes / kind.BytesPerElement()
		}
	case []Value:
		// Creating from array of values
		arrayLength = len(arg)
		bytesNeeded := arrayLength * kind.BytesPerElement()
		newBuffer := &ArrayBufferObject{data: make([]byte, bytesNeeded), maxByteLength: -1}

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
		trackLength: trackLength,
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
