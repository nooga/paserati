package vm

import (
	"encoding/binary"
	"math"
	"math/big"
	"unsafe"
)

// DataViewObject represents a DataView that provides a low-level interface
// for reading and writing multiple number types in an ArrayBuffer
type DataViewObject struct {
	Object
	buffer     BufferData // Can be ArrayBuffer or SharedArrayBuffer
	byteOffset int
	byteLength int
}

// GetBuffer returns the underlying buffer as an ArrayBufferObject
func (dv *DataViewObject) GetBuffer() *ArrayBufferObject {
	if ab, ok := dv.buffer.(*ArrayBufferObject); ok {
		return ab
	}
	return nil
}

// GetBufferData returns the underlying buffer (ArrayBuffer or SharedArrayBuffer)
func (dv *DataViewObject) GetBufferData() BufferData {
	return dv.buffer
}

// IsSharedBuffer returns true if the underlying buffer is a SharedArrayBuffer
func (dv *DataViewObject) IsSharedBuffer() bool {
	_, ok := dv.buffer.(*SharedArrayBufferObject)
	return ok
}

// GetSharedBuffer returns the underlying SharedArrayBuffer, or nil if not shared
func (dv *DataViewObject) GetSharedBuffer() *SharedArrayBufferObject {
	if sab, ok := dv.buffer.(*SharedArrayBufferObject); ok {
		return sab
	}
	return nil
}

// GetByteOffset returns the byte offset into the buffer
func (dv *DataViewObject) GetByteOffset() int {
	return dv.byteOffset
}

// GetByteLength returns the byte length of the view
func (dv *DataViewObject) GetByteLength() int {
	return dv.byteLength
}

// NewDataView creates a new DataView value
func NewDataView(buffer BufferData, byteOffset, byteLength int) Value {
	dv := &DataViewObject{
		buffer:     buffer,
		byteOffset: byteOffset,
		byteLength: byteLength,
	}
	return Value{typ: TypeDataView, obj: unsafe.Pointer(dv)}
}

// AsDataView returns the DataViewObject if the value is a DataView, nil otherwise
func (v Value) AsDataView() *DataViewObject {
	if v.typ == TypeDataView {
		return (*DataViewObject)(v.obj)
	}
	return nil
}

// GetInt8 reads a signed 8-bit integer at the specified byte offset
func (dv *DataViewObject) GetInt8(byteOffset int) (int8, bool) {
	if byteOffset < 0 || byteOffset >= dv.byteLength {
		return 0, false
	}
	if dv.buffer.IsDetached() {
		return 0, false
	}
	data := dv.buffer.GetData()
	return int8(data[dv.byteOffset+byteOffset]), true
}

// GetUint8 reads an unsigned 8-bit integer at the specified byte offset
func (dv *DataViewObject) GetUint8(byteOffset int) (uint8, bool) {
	if byteOffset < 0 || byteOffset >= dv.byteLength {
		return 0, false
	}
	if dv.buffer.IsDetached() {
		return 0, false
	}
	data := dv.buffer.GetData()
	return data[dv.byteOffset+byteOffset], true
}

// GetInt16 reads a signed 16-bit integer at the specified byte offset
func (dv *DataViewObject) GetInt16(byteOffset int, littleEndian bool) (int16, bool) {
	if byteOffset < 0 || byteOffset+2 > dv.byteLength {
		return 0, false
	}
	if dv.buffer.IsDetached() {
		return 0, false
	}
	data := dv.buffer.GetData()[dv.byteOffset+byteOffset:]
	var val uint16
	if littleEndian {
		val = binary.LittleEndian.Uint16(data)
	} else {
		val = binary.BigEndian.Uint16(data)
	}
	return int16(val), true
}

// GetUint16 reads an unsigned 16-bit integer at the specified byte offset
func (dv *DataViewObject) GetUint16(byteOffset int, littleEndian bool) (uint16, bool) {
	if byteOffset < 0 || byteOffset+2 > dv.byteLength {
		return 0, false
	}
	if dv.buffer.IsDetached() {
		return 0, false
	}
	data := dv.buffer.GetData()[dv.byteOffset+byteOffset:]
	if littleEndian {
		return binary.LittleEndian.Uint16(data), true
	}
	return binary.BigEndian.Uint16(data), true
}

// GetInt32 reads a signed 32-bit integer at the specified byte offset
func (dv *DataViewObject) GetInt32(byteOffset int, littleEndian bool) (int32, bool) {
	if byteOffset < 0 || byteOffset+4 > dv.byteLength {
		return 0, false
	}
	if dv.buffer.IsDetached() {
		return 0, false
	}
	data := dv.buffer.GetData()[dv.byteOffset+byteOffset:]
	var val uint32
	if littleEndian {
		val = binary.LittleEndian.Uint32(data)
	} else {
		val = binary.BigEndian.Uint32(data)
	}
	return int32(val), true
}

// GetUint32 reads an unsigned 32-bit integer at the specified byte offset
func (dv *DataViewObject) GetUint32(byteOffset int, littleEndian bool) (uint32, bool) {
	if byteOffset < 0 || byteOffset+4 > dv.byteLength {
		return 0, false
	}
	if dv.buffer.IsDetached() {
		return 0, false
	}
	data := dv.buffer.GetData()[dv.byteOffset+byteOffset:]
	if littleEndian {
		return binary.LittleEndian.Uint32(data), true
	}
	return binary.BigEndian.Uint32(data), true
}

// GetFloat32 reads a 32-bit float at the specified byte offset
func (dv *DataViewObject) GetFloat32(byteOffset int, littleEndian bool) (float32, bool) {
	if byteOffset < 0 || byteOffset+4 > dv.byteLength {
		return 0, false
	}
	if dv.buffer.IsDetached() {
		return 0, false
	}
	data := dv.buffer.GetData()[dv.byteOffset+byteOffset:]
	var bits uint32
	if littleEndian {
		bits = binary.LittleEndian.Uint32(data)
	} else {
		bits = binary.BigEndian.Uint32(data)
	}
	return math.Float32frombits(bits), true
}

// GetFloat64 reads a 64-bit float at the specified byte offset
func (dv *DataViewObject) GetFloat64(byteOffset int, littleEndian bool) (float64, bool) {
	if byteOffset < 0 || byteOffset+8 > dv.byteLength {
		return 0, false
	}
	if dv.buffer.IsDetached() {
		return 0, false
	}
	data := dv.buffer.GetData()[dv.byteOffset+byteOffset:]
	var bits uint64
	if littleEndian {
		bits = binary.LittleEndian.Uint64(data)
	} else {
		bits = binary.BigEndian.Uint64(data)
	}
	return math.Float64frombits(bits), true
}

// GetBigInt64 reads a signed 64-bit integer at the specified byte offset
func (dv *DataViewObject) GetBigInt64(byteOffset int, littleEndian bool) (*big.Int, bool) {
	if byteOffset < 0 || byteOffset+8 > dv.byteLength {
		return nil, false
	}
	if dv.buffer.IsDetached() {
		return nil, false
	}
	data := dv.buffer.GetData()[dv.byteOffset+byteOffset:]
	var val uint64
	if littleEndian {
		val = binary.LittleEndian.Uint64(data)
	} else {
		val = binary.BigEndian.Uint64(data)
	}
	return big.NewInt(int64(val)), true
}

// GetBigUint64 reads an unsigned 64-bit integer at the specified byte offset
func (dv *DataViewObject) GetBigUint64(byteOffset int, littleEndian bool) (*big.Int, bool) {
	if byteOffset < 0 || byteOffset+8 > dv.byteLength {
		return nil, false
	}
	if dv.buffer.IsDetached() {
		return nil, false
	}
	data := dv.buffer.GetData()[dv.byteOffset+byteOffset:]
	var val uint64
	if littleEndian {
		val = binary.LittleEndian.Uint64(data)
	} else {
		val = binary.BigEndian.Uint64(data)
	}
	return new(big.Int).SetUint64(val), true
}

// SetInt8 writes a signed 8-bit integer at the specified byte offset
func (dv *DataViewObject) SetInt8(byteOffset int, value int8) bool {
	if byteOffset < 0 || byteOffset >= dv.byteLength {
		return false
	}
	if dv.buffer.IsDetached() {
		return false
	}
	data := dv.buffer.GetData()
	data[dv.byteOffset+byteOffset] = byte(value)
	return true
}

// SetUint8 writes an unsigned 8-bit integer at the specified byte offset
func (dv *DataViewObject) SetUint8(byteOffset int, value uint8) bool {
	if byteOffset < 0 || byteOffset >= dv.byteLength {
		return false
	}
	if dv.buffer.IsDetached() {
		return false
	}
	data := dv.buffer.GetData()
	data[dv.byteOffset+byteOffset] = value
	return true
}

// SetInt16 writes a signed 16-bit integer at the specified byte offset
func (dv *DataViewObject) SetInt16(byteOffset int, value int16, littleEndian bool) bool {
	if byteOffset < 0 || byteOffset+2 > dv.byteLength {
		return false
	}
	if dv.buffer.IsDetached() {
		return false
	}
	data := dv.buffer.GetData()[dv.byteOffset+byteOffset:]
	if littleEndian {
		binary.LittleEndian.PutUint16(data, uint16(value))
	} else {
		binary.BigEndian.PutUint16(data, uint16(value))
	}
	return true
}

// SetUint16 writes an unsigned 16-bit integer at the specified byte offset
func (dv *DataViewObject) SetUint16(byteOffset int, value uint16, littleEndian bool) bool {
	if byteOffset < 0 || byteOffset+2 > dv.byteLength {
		return false
	}
	if dv.buffer.IsDetached() {
		return false
	}
	data := dv.buffer.GetData()[dv.byteOffset+byteOffset:]
	if littleEndian {
		binary.LittleEndian.PutUint16(data, value)
	} else {
		binary.BigEndian.PutUint16(data, value)
	}
	return true
}

// SetInt32 writes a signed 32-bit integer at the specified byte offset
func (dv *DataViewObject) SetInt32(byteOffset int, value int32, littleEndian bool) bool {
	if byteOffset < 0 || byteOffset+4 > dv.byteLength {
		return false
	}
	if dv.buffer.IsDetached() {
		return false
	}
	data := dv.buffer.GetData()[dv.byteOffset+byteOffset:]
	if littleEndian {
		binary.LittleEndian.PutUint32(data, uint32(value))
	} else {
		binary.BigEndian.PutUint32(data, uint32(value))
	}
	return true
}

// SetUint32 writes an unsigned 32-bit integer at the specified byte offset
func (dv *DataViewObject) SetUint32(byteOffset int, value uint32, littleEndian bool) bool {
	if byteOffset < 0 || byteOffset+4 > dv.byteLength {
		return false
	}
	if dv.buffer.IsDetached() {
		return false
	}
	data := dv.buffer.GetData()[dv.byteOffset+byteOffset:]
	if littleEndian {
		binary.LittleEndian.PutUint32(data, value)
	} else {
		binary.BigEndian.PutUint32(data, value)
	}
	return true
}

// SetFloat32 writes a 32-bit float at the specified byte offset
func (dv *DataViewObject) SetFloat32(byteOffset int, value float32, littleEndian bool) bool {
	if byteOffset < 0 || byteOffset+4 > dv.byteLength {
		return false
	}
	if dv.buffer.IsDetached() {
		return false
	}
	data := dv.buffer.GetData()[dv.byteOffset+byteOffset:]
	bits := math.Float32bits(value)
	if littleEndian {
		binary.LittleEndian.PutUint32(data, bits)
	} else {
		binary.BigEndian.PutUint32(data, bits)
	}
	return true
}

// SetFloat64 writes a 64-bit float at the specified byte offset
func (dv *DataViewObject) SetFloat64(byteOffset int, value float64, littleEndian bool) bool {
	if byteOffset < 0 || byteOffset+8 > dv.byteLength {
		return false
	}
	if dv.buffer.IsDetached() {
		return false
	}
	data := dv.buffer.GetData()[dv.byteOffset+byteOffset:]
	bits := math.Float64bits(value)
	if littleEndian {
		binary.LittleEndian.PutUint64(data, bits)
	} else {
		binary.BigEndian.PutUint64(data, bits)
	}
	return true
}

// SetBigInt64 writes a signed 64-bit integer at the specified byte offset
func (dv *DataViewObject) SetBigInt64(byteOffset int, value *big.Int, littleEndian bool) bool {
	if byteOffset < 0 || byteOffset+8 > dv.byteLength {
		return false
	}
	if dv.buffer.IsDetached() {
		return false
	}
	data := dv.buffer.GetData()[dv.byteOffset+byteOffset:]
	val := uint64(value.Int64())
	if littleEndian {
		binary.LittleEndian.PutUint64(data, val)
	} else {
		binary.BigEndian.PutUint64(data, val)
	}
	return true
}

// SetBigUint64 writes an unsigned 64-bit integer at the specified byte offset
func (dv *DataViewObject) SetBigUint64(byteOffset int, value *big.Int, littleEndian bool) bool {
	if byteOffset < 0 || byteOffset+8 > dv.byteLength {
		return false
	}
	if dv.buffer.IsDetached() {
		return false
	}
	data := dv.buffer.GetData()[dv.byteOffset+byteOffset:]
	val := value.Uint64()
	if littleEndian {
		binary.LittleEndian.PutUint64(data, val)
	} else {
		binary.BigEndian.PutUint64(data, val)
	}
	return true
}
