package builtins

import (
	"github.com/nooga/paserati/pkg/types"
	"github.com/nooga/paserati/pkg/vm"
)

// Priority constant for Blob
const PriorityBlob = 190 // Before fetch

type BlobInitializer struct{}

func (b *BlobInitializer) Name() string {
	return "Blob"
}

func (b *BlobInitializer) Priority() int {
	return PriorityBlob
}

func (b *BlobInitializer) InitTypes(ctx *TypeContext) error {
	// Blob type
	blobType := types.NewObjectType().
		WithProperty("size", types.Number).
		WithProperty("type", types.String).
		WithProperty("arrayBuffer", types.NewSimpleFunction([]types.Type{}, types.Any)). // Returns Promise<ArrayBuffer>
		WithProperty("bytes", types.NewSimpleFunction([]types.Type{}, types.Any)).       // Returns Promise<Uint8Array>
		WithProperty("text", types.NewSimpleFunction([]types.Type{}, types.Any)).        // Returns Promise<string>
		WithProperty("slice", types.NewSimpleFunction([]types.Type{types.Number, types.Number, types.String}, types.Any)).
		WithProperty("stream", types.NewSimpleFunction([]types.Type{}, types.Any)) // Returns ReadableStream (stub)

	// BlobPropertyBag type
	blobOptionsType := types.NewObjectType().
		WithOptionalProperty("type", types.String).
		WithOptionalProperty("endings", types.String) // "transparent" | "native"

	// Blob constructor type
	blobConstructorType := types.NewObjectType().
		WithSimpleCallSignature([]types.Type{}, blobType).                            // Blob()
		WithSimpleCallSignature([]types.Type{types.Any}, blobType).                   // Blob(blobParts)
		WithSimpleCallSignature([]types.Type{types.Any, blobOptionsType}, blobType).  // Blob(blobParts, options)
		WithProperty("prototype", blobType)

	return ctx.DefineGlobal("Blob", blobConstructorType)
}

func (b *BlobInitializer) InitRuntime(ctx *RuntimeContext) error {
	vmInstance := ctx.VM

	// Create Blob.prototype
	blobProto := vm.NewObject(vmInstance.ObjectPrototype).AsPlainObject()

	// Create Blob constructor
	blobConstructorFn := func(args []vm.Value) (vm.Value, error) {
		blob := &Blob{
			data:     []byte{},
			mimeType: "",
		}

		// Parse blobParts if provided
		if len(args) > 0 && args[0].Type() != vm.TypeUndefined && args[0].Type() != vm.TypeNull {
			parts := args[0]
			if arr := parts.AsArray(); arr != nil {
				for i := 0; i < arr.Length(); i++ {
					part := arr.Get(i)
					blob.data = append(blob.data, blobPartToBytes(part)...)
				}
			} else {
				// Single value
				blob.data = blobPartToBytes(parts)
			}
		}

		// Parse options if provided
		if len(args) > 1 && args[1].Type() != vm.TypeUndefined && args[1].Type() != vm.TypeNull {
			if opts := args[1].AsPlainObject(); opts != nil {
				if t, exists := opts.GetOwn("type"); exists && t.Type() == vm.TypeString {
					blob.mimeType = t.ToString()
				}
			} else if opts := args[1].AsDictObject(); opts != nil {
				if t, exists := opts.GetOwn("type"); exists && t.Type() == vm.TypeString {
					blob.mimeType = t.ToString()
				}
			}
		}

		return createBlobObject(vmInstance, blob, blobProto), nil
	}

	blobConstructor := vm.NewConstructorWithProps(2, false, "Blob", blobConstructorFn)
	if ctorProps := blobConstructor.AsNativeFunctionWithProps(); ctorProps != nil {
		ctorProps.Properties.SetOwnNonEnumerable("prototype", vm.NewValueFromPlainObject(blobProto))
	}

	blobProto.SetOwnNonEnumerable("constructor", blobConstructor)

	return ctx.DefineGlobal("Blob", blobConstructor)
}

// Blob represents binary data with a MIME type
type Blob struct {
	data     []byte
	mimeType string
}

// blobPartToBytes converts various types to bytes for Blob construction
func blobPartToBytes(part vm.Value) []byte {
	switch part.Type() {
	case vm.TypeString:
		return []byte(part.ToString())
	case vm.TypeArrayBuffer:
		if buf := part.AsArrayBuffer(); buf != nil {
			data := make([]byte, len(buf.GetData()))
			copy(data, buf.GetData())
			return data
		}
	case vm.TypeTypedArray:
		if ta := part.AsTypedArray(); ta != nil {
			data := make([]byte, ta.GetLength())
			for i := 0; i < ta.GetLength(); i++ {
				data[i] = byte(ta.GetElement(i).ToFloat())
			}
			return data
		}
	case vm.TypeObject:
		// Check if it's a Blob-like object with data
		if obj := part.AsPlainObject(); obj != nil {
			// Try to get internal blob data (would need special handling)
			// For now, convert to string
			return []byte(part.ToString())
		}
	default:
		return []byte(part.ToString())
	}
	return []byte{}
}

func createBlobObject(vmInstance *vm.VM, blob *Blob, _ *vm.PlainObject) vm.Value {
	obj := vm.NewObject(vmInstance.ObjectPrototype).AsPlainObject()

	// size property (read-only)
	obj.SetOwn("size", vm.NumberValue(float64(len(blob.data))))

	// type property (read-only)
	obj.SetOwn("type", vm.NewString(blob.mimeType))

	// arrayBuffer() -> Promise<ArrayBuffer>
	obj.SetOwnNonEnumerable("arrayBuffer", vm.NewNativeFunction(0, false, "arrayBuffer", func(args []vm.Value) (vm.Value, error) {
		arrayBuffer := vm.NewArrayBuffer(len(blob.data))
		buf := arrayBuffer.AsArrayBuffer()
		copy(buf.GetData(), blob.data)
		return vmInstance.NewResolvedPromise(arrayBuffer), nil
	}))

	// bytes() -> Promise<Uint8Array>
	obj.SetOwnNonEnumerable("bytes", vm.NewNativeFunction(0, false, "bytes", func(args []vm.Value) (vm.Value, error) {
		arrayBuffer := vm.NewArrayBuffer(len(blob.data))
		buf := arrayBuffer.AsArrayBuffer()
		copy(buf.GetData(), blob.data)
		uint8Array := vm.NewTypedArray(vm.TypedArrayUint8, buf, 0, 0)
		return vmInstance.NewResolvedPromise(uint8Array), nil
	}))

	// text() -> Promise<string>
	obj.SetOwnNonEnumerable("text", vm.NewNativeFunction(0, false, "text", func(args []vm.Value) (vm.Value, error) {
		return vmInstance.NewResolvedPromise(vm.NewString(string(blob.data))), nil
	}))

	// slice(start?, end?, contentType?) -> Blob
	obj.SetOwnNonEnumerable("slice", vm.NewNativeFunction(3, false, "slice", func(args []vm.Value) (vm.Value, error) {
		start := 0
		end := len(blob.data)
		contentType := blob.mimeType

		if len(args) > 0 && args[0].IsNumber() {
			start = int(args[0].ToFloat())
			if start < 0 {
				start = len(blob.data) + start
				if start < 0 {
					start = 0
				}
			}
		}

		if len(args) > 1 && args[1].IsNumber() {
			end = int(args[1].ToFloat())
			if end < 0 {
				end = len(blob.data) + end
			}
			if end > len(blob.data) {
				end = len(blob.data)
			}
		}

		if len(args) > 2 && args[2].Type() == vm.TypeString {
			contentType = args[2].ToString()
		}

		if start > end {
			start = end
		}

		newBlob := &Blob{
			data:     blob.data[start:end],
			mimeType: contentType,
		}

		return createBlobObject(vmInstance, newBlob, nil), nil
	}))

	// stream() -> ReadableStream (stub - returns undefined for now)
	obj.SetOwnNonEnumerable("stream", vm.NewNativeFunction(0, false, "stream", func(args []vm.Value) (vm.Value, error) {
		// ReadableStream would require significant infrastructure
		return vm.Undefined, nil
	}))

	return vm.NewValueFromPlainObject(obj)
}
