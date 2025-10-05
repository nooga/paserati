package vm

import (
	"encoding/binary"
	"math"
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
	data     []byte
	detached bool
}

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

// TypedArrayObject represents a typed view into an ArrayBuffer
type TypedArrayObject struct {
	Object
	buffer      *ArrayBufferObject
	byteOffset  int
	byteLength  int
	length      int // number of elements
	elementType TypedArrayKind
}

// Getter methods for TypedArrayObject
func (ta *TypedArrayObject) GetBuffer() *ArrayBufferObject {
	return ta.buffer
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
	data := ta.buffer.data[offset:]

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
	default:
		// BigInt cases would go here
		return Undefined
	}
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
	data := ta.buffer.data[offset:]

	switch ta.elementType {
	case TypedArrayInt8:
		data[0] = byte(int8(num))
	case TypedArrayUint8:
		data[0] = byte(uint8(num))
	case TypedArrayUint8Clamped:
		// Clamp between 0 and 255
		if num < 0 {
			data[0] = 0
		} else if num > 255 {
			data[0] = 255
		} else {
			data[0] = byte(num)
		}
	case TypedArrayInt16:
		binary.LittleEndian.PutUint16(data, uint16(int16(num)))
	case TypedArrayUint16:
		binary.LittleEndian.PutUint16(data, uint16(num))
	case TypedArrayInt32:
		// JavaScript-style int32 conversion with proper wrapping
		val := int64(num) // Convert to int64 first to handle large numbers
		wrapped := int32(val) // This will wrap correctly
		binary.LittleEndian.PutUint32(data, uint32(wrapped))
	case TypedArrayUint32:
		binary.LittleEndian.PutUint32(data, uint32(num))
	case TypedArrayFloat32:
		binary.LittleEndian.PutUint32(data, math.Float32bits(float32(num)))
	case TypedArrayFloat64:
		binary.LittleEndian.PutUint64(data, math.Float64bits(num))
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

func NewTypedArray(kind TypedArrayKind, lengthOrBuffer interface{}, byteOffset, length int) Value {
	var buffer *ArrayBufferObject
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
		// Creating from existing buffer
		buffer = arg
		arrayByteOffset = byteOffset
		if length > 0 {
			arrayLength = length
		} else {
			// Calculate length from buffer size
			remainingBytes := len(buffer.data) - arrayByteOffset
			arrayLength = remainingBytes / kind.BytesPerElement()
		}
	case []Value:
		// Creating from array of values
		arrayLength = len(arg)
		bytesNeeded := arrayLength * kind.BytesPerElement()
		buffer = &ArrayBufferObject{data: make([]byte, bytesNeeded)}
		arrayByteOffset = 0
		
		// Initialize with values
		ta := &TypedArrayObject{
			buffer:      buffer,
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

func (v Value) AsTypedArray() *TypedArrayObject {
	if v.typ == TypeTypedArray {
		return (*TypedArrayObject)(v.obj)
	}
	return nil
}