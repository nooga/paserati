package builtins

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/nooga/paserati/pkg/types"
	"github.com/nooga/paserati/pkg/vm"
)

// Priority constant for fetch
const PriorityFetch = 200 // After most builtins

type FetchInitializer struct{}

func (f *FetchInitializer) Name() string {
	return "fetch"
}

func (f *FetchInitializer) Priority() int {
	return PriorityFetch
}

func (f *FetchInitializer) InitTypes(ctx *TypeContext) error {
	// Headers type (prototype) with iterator methods
	headersType := types.NewObjectType().
		WithProperty("get", types.NewSimpleFunction([]types.Type{types.String}, types.String)).
		WithProperty("has", types.NewSimpleFunction([]types.Type{types.String}, types.Boolean)).
		WithProperty("set", types.NewSimpleFunction([]types.Type{types.String, types.String}, types.Undefined)).
		WithProperty("delete", types.NewSimpleFunction([]types.Type{types.String}, types.Undefined)).
		WithProperty("append", types.NewSimpleFunction([]types.Type{types.String, types.String}, types.Undefined)).
		WithProperty("entries", types.NewSimpleFunction([]types.Type{}, types.Any)).
		WithProperty("keys", types.NewSimpleFunction([]types.Type{}, types.Any)).
		WithProperty("values", types.NewSimpleFunction([]types.Type{}, types.Any)).
		WithProperty("forEach", types.NewSimpleFunction([]types.Type{types.Any}, types.Undefined))

	// Headers constructor type (callable with new)
	headersConstructorType := types.NewObjectType().
		WithSimpleCallSignature([]types.Type{}, headersType).                      // Headers()
		WithSimpleCallSignature([]types.Type{types.NewObjectType()}, headersType). // Headers(init)
		WithProperty("prototype", headersType)

	if err := ctx.DefineGlobal("Headers", headersConstructorType); err != nil {
		return err
	}

	// Response type with all standard properties and methods
	responseType := types.NewObjectType().
		WithProperty("ok", types.Boolean).
		WithProperty("status", types.Number).
		WithProperty("statusText", types.String).
		WithProperty("url", types.String).
		WithProperty("headers", headersType).
		WithProperty("bodyUsed", types.Boolean).
		WithProperty("redirected", types.Boolean).
		WithProperty("type", types.String).
		WithProperty("text", types.NewSimpleFunction([]types.Type{}, types.Any)).        // Returns Promise<string>
		WithProperty("json", types.NewSimpleFunction([]types.Type{}, types.Any)).        // Returns Promise<any>
		WithProperty("blob", types.NewSimpleFunction([]types.Type{}, types.Any)).        // Returns Promise<Uint8Array>
		WithProperty("arrayBuffer", types.NewSimpleFunction([]types.Type{}, types.Any)). // Returns Promise<ArrayBuffer>
		WithProperty("bytes", types.NewSimpleFunction([]types.Type{}, types.Any)).       // Returns Promise<Uint8Array>
		WithProperty("clone", types.NewSimpleFunction([]types.Type{}, types.Any))        // Returns Response

	// ResponseInit type
	responseInitType := types.NewObjectType().
		WithOptionalProperty("status", types.Number).
		WithOptionalProperty("statusText", types.String).
		WithOptionalProperty("headers", types.NewUnionType(headersType, types.NewObjectType()))

	// Response constructor type
	responseConstructorType := types.NewObjectType().
		WithSimpleCallSignature([]types.Type{}, responseType).                                    // Response()
		WithSimpleCallSignature([]types.Type{types.Any}, responseType).                           // Response(body)
		WithSimpleCallSignature([]types.Type{types.Any, responseInitType}, responseType).         // Response(body, init)
		WithProperty("prototype", responseType).
		WithProperty("error", types.NewSimpleFunction([]types.Type{}, responseType)).             // Response.error()
		WithProperty("redirect", types.NewSimpleFunction([]types.Type{types.String, types.Number}, responseType)). // Response.redirect(url, status)
		WithProperty("json", types.NewSimpleFunction([]types.Type{types.Any, responseInitType}, responseType))     // Response.json(data, init)

	if err := ctx.DefineGlobal("Response", responseConstructorType); err != nil {
		return err
	}

	// RequestInit type with all standard options
	requestInitType := types.NewObjectType().
		WithOptionalProperty("method", types.String).
		WithOptionalProperty("headers", types.NewUnionType(headersType, types.NewObjectType())).
		WithOptionalProperty("body", types.NewUnionType(types.String, types.NewObjectType())).
		WithOptionalProperty("signal", types.Any).      // AbortSignal
		WithOptionalProperty("redirect", types.String). // "follow" | "error" | "manual"
		WithOptionalProperty("credentials", types.String). // "omit" | "same-origin" | "include"
		WithOptionalProperty("cache", types.String).       // cache mode
		WithOptionalProperty("mode", types.String).        // CORS mode
		WithOptionalProperty("referrer", types.String).
		WithOptionalProperty("referrerPolicy", types.String).
		WithOptionalProperty("keepalive", types.Boolean)

	// Request type with all standard properties and methods
	requestType := types.NewObjectType().
		WithProperty("method", types.String).
		WithProperty("url", types.String).
		WithProperty("headers", headersType).
		WithProperty("body", types.Any). // ReadableStream or null
		WithProperty("bodyUsed", types.Boolean).
		WithProperty("cache", types.String).
		WithProperty("credentials", types.String).
		WithProperty("destination", types.String).
		WithProperty("integrity", types.String).
		WithProperty("mode", types.String).
		WithProperty("redirect", types.String).
		WithProperty("referrer", types.String).
		WithProperty("referrerPolicy", types.String).
		WithProperty("signal", types.Any). // AbortSignal
		WithProperty("clone", types.NewSimpleFunction([]types.Type{}, types.Any)).        // Returns Request
		WithProperty("arrayBuffer", types.NewSimpleFunction([]types.Type{}, types.Any)).  // Returns Promise<ArrayBuffer>
		WithProperty("blob", types.NewSimpleFunction([]types.Type{}, types.Any)).         // Returns Promise<Blob>
		WithProperty("formData", types.NewSimpleFunction([]types.Type{}, types.Any)).     // Returns Promise<FormData>
		WithProperty("json", types.NewSimpleFunction([]types.Type{}, types.Any)).         // Returns Promise<any>
		WithProperty("text", types.NewSimpleFunction([]types.Type{}, types.Any))          // Returns Promise<string>

	// Request constructor type
	requestConstructorType := types.NewObjectType().
		WithSimpleCallSignature([]types.Type{types.String}, requestType).                 // Request(url)
		WithSimpleCallSignature([]types.Type{types.String, requestInitType}, requestType). // Request(url, init)
		WithSimpleCallSignature([]types.Type{requestType}, requestType).                  // Request(request)
		WithSimpleCallSignature([]types.Type{requestType, requestInitType}, requestType). // Request(request, init)
		WithProperty("prototype", requestType)

	if err := ctx.DefineGlobal("Request", requestConstructorType); err != nil {
		return err
	}

	// fetch function type: (url: string | Request, init?: RequestInit) => Promise<Response>
	// Second parameter is optional
	fetchType := types.NewOptionalFunction(
		[]types.Type{types.NewUnionType(types.String, requestType), requestInitType},
		types.Any,
		[]bool{false, true}, // url/request is required, init is optional
	)

	return ctx.DefineGlobal("fetch", fetchType)
}

func (f *FetchInitializer) InitRuntime(ctx *RuntimeContext) error {
	vmInstance := ctx.VM

	// Create Headers.prototype
	headersProto := vm.NewObject(vmInstance.ObjectPrototype).AsPlainObject()

	// Create Headers constructor as a proper constructor with new support
	headersConstructorFn := func(args []vm.Value) (vm.Value, error) {
		headers := &FetchHeaders{headers: make(http.Header)}

		// Initialize with headers if provided
		if len(args) > 0 && args[0].Type() != vm.TypeUndefined && args[0].Type() != vm.TypeNull {
			if args[0].Type() == vm.TypeObject {
				obj := args[0].AsPlainObject()
				keys := obj.OwnKeys()
				for _, key := range keys {
					if val, exists := obj.GetOwn(key); exists {
						headers.headers.Set(key, val.ToString())
					}
				}
			} else if args[0].Type() == vm.TypeDictObject {
				dictObj := args[0].AsDictObject()
				keys := dictObj.OwnKeys()
				for _, key := range keys {
					if val, exists := dictObj.GetOwn(key); exists {
						headers.headers.Set(key, val.ToString())
					}
				}
			}
		}

		return createHeadersObject(vmInstance, headers), nil
	}

	// Use NewConstructorWithProps to make it callable with 'new'
	headersConstructor := vm.NewConstructorWithProps(1, false, "Headers", headersConstructorFn)
	if ctorProps := headersConstructor.AsNativeFunctionWithProps(); ctorProps != nil {
		ctorProps.Properties.SetOwnNonEnumerable("prototype", vm.NewValueFromPlainObject(headersProto))
	}

	// Set constructor on prototype
	headersProto.SetOwnNonEnumerable("constructor", headersConstructor)

	if err := ctx.DefineGlobal("Headers", headersConstructor); err != nil {
		return err
	}

	// Create Response.prototype
	responseProto := vm.NewObject(vmInstance.ObjectPrototype).AsPlainObject()

	// Create Response constructor
	responseConstructorFn := func(args []vm.Value) (vm.Value, error) {
		var bodyBytes []byte
		status := 200
		statusText := "OK"
		headers := &FetchHeaders{headers: make(http.Header)}

		// Parse body if provided
		if len(args) > 0 && args[0].Type() != vm.TypeUndefined && args[0].Type() != vm.TypeNull {
			bodyBytes = valueToBytes(args[0])
		}

		// Parse init options if provided
		if len(args) > 1 && args[1].Type() != vm.TypeUndefined && args[1].Type() != vm.TypeNull {
			if args[1].Type() == vm.TypeObject {
				initObj := args[1].AsPlainObject()
				if s, exists := initObj.GetOwn("status"); exists && s.IsNumber() {
					status = int(s.ToFloat())
				}
				if st, exists := initObj.GetOwn("statusText"); exists && st.Type() == vm.TypeString {
					statusText = st.ToString()
				}
				if h, exists := initObj.GetOwn("headers"); exists && h.Type() != vm.TypeUndefined {
					headers = parseHeaders(h)
				}
			} else if args[1].Type() == vm.TypeDictObject {
				initObj := args[1].AsDictObject()
				if s, exists := initObj.GetOwn("status"); exists && s.IsNumber() {
					status = int(s.ToFloat())
				}
				if st, exists := initObj.GetOwn("statusText"); exists && st.Type() == vm.TypeString {
					statusText = st.ToString()
				}
				if h, exists := initObj.GetOwn("headers"); exists && h.Type() != vm.TypeUndefined {
					headers = parseHeaders(h)
				}
			}
		}

		response := &FetchResponse{
			vm:         vmInstance,
			OK:         status >= 200 && status < 300,
			Status:     status,
			StatusText: statusText,
			URL:        "",
			Headers:    headers,
			body:       bodyBytes,
			bodyUsed:   false,
			Redirected: false,
			Type:       "default",
		}

		return createResponseObject(vmInstance, response), nil
	}

	responseConstructor := vm.NewConstructorWithProps(2, false, "Response", responseConstructorFn)
	if ctorProps := responseConstructor.AsNativeFunctionWithProps(); ctorProps != nil {
		ctorProps.Properties.SetOwnNonEnumerable("prototype", vm.NewValueFromPlainObject(responseProto))

		// Response.error() - returns an error response
		ctorProps.Properties.SetOwnNonEnumerable("error", vm.NewNativeFunction(0, false, "error", func(args []vm.Value) (vm.Value, error) {
			response := &FetchResponse{
				vm:         vmInstance,
				OK:         false,
				Status:     0,
				StatusText: "",
				URL:        "",
				Headers:    &FetchHeaders{headers: make(http.Header)},
				body:       []byte{},
				bodyUsed:   false,
				Redirected: false,
				Type:       "error",
			}
			return createResponseObject(vmInstance, response), nil
		}))

		// Response.redirect(url, status?) - returns a redirect response
		ctorProps.Properties.SetOwnNonEnumerable("redirect", vm.NewNativeFunction(2, false, "redirect", func(args []vm.Value) (vm.Value, error) {
			if len(args) < 1 {
				return vm.Undefined, fmt.Errorf("TypeError: Response.redirect requires a URL")
			}
			url := args[0].ToString()
			status := 302 // Default redirect status
			if len(args) > 1 && args[1].IsNumber() {
				status = int(args[1].ToFloat())
			}
			// Validate redirect status
			if status != 301 && status != 302 && status != 303 && status != 307 && status != 308 {
				return vm.Undefined, fmt.Errorf("RangeError: Invalid redirect status code")
			}
			headers := &FetchHeaders{headers: make(http.Header)}
			headers.headers.Set("Location", url)
			response := &FetchResponse{
				vm:         vmInstance,
				OK:         false,
				Status:     status,
				StatusText: http.StatusText(status),
				URL:        "",
				Headers:    headers,
				body:       []byte{},
				bodyUsed:   false,
				Redirected: false,
				Type:       "default",
			}
			return createResponseObject(vmInstance, response), nil
		}))

		// Response.json(data, init?) - returns a response with JSON body
		ctorProps.Properties.SetOwnNonEnumerable("json", vm.NewNativeFunction(2, false, "json", func(args []vm.Value) (vm.Value, error) {
			if len(args) < 1 {
				return vm.Undefined, fmt.Errorf("TypeError: Response.json requires data")
			}
			jsonBytes, err := args[0].MarshalJSON()
			if err != nil {
				return vm.Undefined, fmt.Errorf("TypeError: Failed to serialize data to JSON")
			}

			status := 200
			statusText := "OK"
			headers := &FetchHeaders{headers: make(http.Header)}
			headers.headers.Set("Content-Type", "application/json")

			// Parse init options if provided
			if len(args) > 1 && args[1].Type() != vm.TypeUndefined && args[1].Type() != vm.TypeNull {
				if args[1].Type() == vm.TypeObject {
					initObj := args[1].AsPlainObject()
					if s, exists := initObj.GetOwn("status"); exists && s.IsNumber() {
						status = int(s.ToFloat())
					}
					if st, exists := initObj.GetOwn("statusText"); exists && st.Type() == vm.TypeString {
						statusText = st.ToString()
					}
					if h, exists := initObj.GetOwn("headers"); exists && h.Type() != vm.TypeUndefined {
						headers = parseHeaders(h)
						// Ensure Content-Type is set for JSON
						if headers.headers.Get("Content-Type") == "" {
							headers.headers.Set("Content-Type", "application/json")
						}
					}
				}
			}

			response := &FetchResponse{
				vm:         vmInstance,
				OK:         status >= 200 && status < 300,
				Status:     status,
				StatusText: statusText,
				URL:        "",
				Headers:    headers,
				body:       jsonBytes,
				bodyUsed:   false,
				Redirected: false,
				Type:       "default",
			}
			return createResponseObject(vmInstance, response), nil
		}))
	}

	responseProto.SetOwnNonEnumerable("constructor", responseConstructor)

	if err := ctx.DefineGlobal("Response", responseConstructor); err != nil {
		return err
	}

	// Create Request.prototype
	requestProto := vm.NewObject(vmInstance.ObjectPrototype).AsPlainObject()

	// Create Request constructor
	requestConstructorFn := func(args []vm.Value) (vm.Value, error) {
		if len(args) < 1 {
			return vm.Undefined, fmt.Errorf("TypeError: Request constructor requires at least 1 argument")
		}

		req := &FetchRequest{
			vm:             vmInstance,
			Method:         "GET",
			URL:            "",
			Headers:        &FetchHeaders{headers: make(http.Header)},
			body:           nil,
			bodyUsed:       false,
			Cache:          "default",
			Credentials:    "same-origin",
			Destination:    "",
			Integrity:      "",
			Mode:           "cors",
			Redirect:       "follow",
			Referrer:       "about:client",
			ReferrerPolicy: "",
			Signal:         vm.Undefined,
		}

		// Check if first argument is a Request object or URL string
		input := args[0]
		if input.Type() == vm.TypeObject {
			inputObj := input.AsPlainObject()
			// Check if it's a Request object by looking for the url property
			if urlVal, exists := inputObj.GetOwn("url"); exists && urlVal.Type() == vm.TypeString {
				// Clone from existing Request
				req.URL = urlVal.ToString()
				if m, exists := inputObj.GetOwn("method"); exists {
					req.Method = m.ToString()
				}
				if h, exists := inputObj.GetOwn("headers"); exists {
					req.Headers = parseHeaders(h)
				}
				if c, exists := inputObj.GetOwn("cache"); exists {
					req.Cache = c.ToString()
				}
				if c, exists := inputObj.GetOwn("credentials"); exists {
					req.Credentials = c.ToString()
				}
				if m, exists := inputObj.GetOwn("mode"); exists {
					req.Mode = m.ToString()
				}
				if r, exists := inputObj.GetOwn("redirect"); exists {
					req.Redirect = r.ToString()
				}
				if r, exists := inputObj.GetOwn("referrer"); exists {
					req.Referrer = r.ToString()
				}
				if r, exists := inputObj.GetOwn("referrerPolicy"); exists {
					req.ReferrerPolicy = r.ToString()
				}
				if s, exists := inputObj.GetOwn("signal"); exists {
					req.Signal = s
				}
			} else {
				req.URL = input.ToString()
			}
		} else {
			req.URL = input.ToString()
		}

		// Parse init options if provided
		if len(args) > 1 && args[1].Type() != vm.TypeUndefined && args[1].Type() != vm.TypeNull {
			if args[1].Type() == vm.TypeObject {
				parseRequestInit(req, args[1].AsPlainObject())
			} else if args[1].Type() == vm.TypeDictObject {
				parseRequestInitDict(req, args[1].AsDictObject())
			}
		}

		return createRequestObject(vmInstance, req, requestProto), nil
	}

	requestConstructor := vm.NewConstructorWithProps(2, false, "Request", requestConstructorFn)
	if ctorProps := requestConstructor.AsNativeFunctionWithProps(); ctorProps != nil {
		ctorProps.Properties.SetOwnNonEnumerable("prototype", vm.NewValueFromPlainObject(requestProto))
	}

	requestProto.SetOwnNonEnumerable("constructor", requestConstructor)

	if err := ctx.DefineGlobal("Request", requestConstructor); err != nil {
		return err
	}

	// Create fetch function - truly async via goroutines
	fetchFn := vm.NewNativeFunction(2, false, "fetch", func(args []vm.Value) (vm.Value, error) {
		if len(args) < 1 {
			return vm.Undefined, fmt.Errorf("TypeError: fetch requires at least 1 argument")
		}

		url := args[0].ToString()
		var init vm.Value = vm.Undefined
		if len(args) > 1 {
			init = args[1]
		}

		// Check for pre-aborted signal synchronously before spawning goroutine
		// This avoids race conditions with very fast abort checks
		if init.Type() != vm.TypeUndefined && init.Type() != vm.TypeNull {
			var initObj interface {
				GetOwn(string) (vm.Value, bool)
			}
			if init.Type() == vm.TypeObject {
				initObj = init.AsPlainObject()
			} else if init.Type() == vm.TypeDictObject {
				initObj = init.AsDictObject()
			}
			if initObj != nil {
				if s, exists := initObj.GetOwn("signal"); exists && s.Type() != vm.TypeUndefined && s.Type() == vm.TypeObject {
					signalObj := s.AsPlainObject()
					if aborted, exists := signalObj.GetOwn("aborted"); exists {
						if aborted.IsBoolean() && aborted.AsBoolean() {
							// Signal is already aborted - reject immediately without async
							reason := "AbortError: signal is aborted without reason"
							if r, exists := signalObj.GetOwn("reason"); exists && r.Type() != vm.TypeUndefined {
								reason = "AbortError: " + r.ToString()
							}
							promise := vmInstance.NewPendingPromise()
							promiseObj := promise.AsPromise()
							vmInstance.RejectPromise(promiseObj, vm.NewString(reason))
							return promise, nil
						}
					}
				}
			}
		}

		// Create pending promise
		promise := vmInstance.NewPendingPromise()
		promiseObj := promise.AsPromise()

		// Get the async runtime to track external operations
		rt := vmInstance.GetAsyncRuntime()

		// Mark that we're starting an external async operation
		rt.BeginExternalOp()

		// Create a cancellable context for the request
		ctx, cancel := context.WithCancel(context.Background())

		// Extract signal for abort monitoring
		var signalObj *vm.PlainObject
		if init.Type() != vm.TypeUndefined && init.Type() != vm.TypeNull {
			var initObj interface {
				GetOwn(string) (vm.Value, bool)
			}
			if init.Type() == vm.TypeObject {
				initObj = init.AsPlainObject()
			} else if init.Type() == vm.TypeDictObject {
				initObj = init.AsDictObject()
			}
			if initObj != nil {
				if s, exists := initObj.GetOwn("signal"); exists && s.Type() == vm.TypeObject {
					signalObj = s.AsPlainObject()
				}
			}
		}

		// If we have a signal, set up abort monitoring
		var abortOnce sync.Once
		if signalObj != nil {
			// Start a goroutine to poll for abort
			// This is a simple polling approach - a more sophisticated approach
			// would use event listeners on the signal
			go func() {
				ticker := time.NewTicker(10 * time.Millisecond)
				defer ticker.Stop()
				for {
					select {
					case <-ctx.Done():
						return
					case <-ticker.C:
						if aborted, exists := signalObj.GetOwn("aborted"); exists {
							if aborted.IsBoolean() && aborted.AsBoolean() {
								abortOnce.Do(func() {
									cancel()
								})
								return
							}
						}
					}
				}
			}()
		}

		// Perform HTTP request asynchronously in a goroutine
		go func() {
			defer cancel() // Clean up context when done

			response, err := doFetchRequestWithContext(ctx, vmInstance, url, init)

			if err != nil {
				// Check if this was a context cancellation (abort)
				if ctx.Err() == context.Canceled {
					reason := "AbortError: The operation was aborted"
					if signalObj != nil {
						if r, exists := signalObj.GetOwn("reason"); exists && r.Type() != vm.TypeUndefined {
							reason = "AbortError: " + r.ToString()
						}
					}
					vmInstance.RejectPromise(promiseObj, vm.NewString(reason))
				} else {
					vmInstance.RejectPromise(promiseObj, vm.NewString(err.Error()))
				}
			} else {
				vmInstance.ResolvePromise(promiseObj, response)
			}

			// Mark that the external operation is complete
			rt.EndExternalOp()
		}()

		return promise, nil
	})

	return ctx.DefineGlobal("fetch", fetchFn)
}

// FetchHeaders wraps http.Header for use in fetch API
type FetchHeaders struct {
	headers http.Header
}

// FetchResponse represents the Response object with VM reference for async methods
type FetchResponse struct {
	vm          *vm.VM
	OK          bool
	Status      int
	StatusText  string
	URL         string
	Headers     *FetchHeaders
	body        []byte
	bodyUsed    bool
	Redirected  bool   // Whether this response is the result of a redirect
	Type        string // Response type: "basic", "cors", "default", "error", "opaque", "opaqueredirect"
}

// boolToValue converts a bool to vm.Value
func boolToValue(b bool) vm.Value {
	if b {
		return vm.True
	}
	return vm.False
}

// createHeadersObject creates a Headers object for the VM
func createHeadersObject(vmInstance *vm.VM, h *FetchHeaders) vm.Value {
	obj := vm.NewObject(vmInstance.ObjectPrototype).AsPlainObject()

	obj.SetOwnNonEnumerable("get", vm.NewNativeFunction(1, false, "get", func(args []vm.Value) (vm.Value, error) {
		if len(args) < 1 {
			return vm.NewString(""), nil
		}
		name := args[0].ToString()
		return vm.NewString(h.headers.Get(name)), nil
	}))

	obj.SetOwnNonEnumerable("has", vm.NewNativeFunction(1, false, "has", func(args []vm.Value) (vm.Value, error) {
		if len(args) < 1 {
			return vm.False, nil
		}
		name := args[0].ToString()
		return boolToValue(h.headers.Get(name) != ""), nil
	}))

	obj.SetOwnNonEnumerable("set", vm.NewNativeFunction(2, false, "set", func(args []vm.Value) (vm.Value, error) {
		if len(args) < 2 {
			return vm.Undefined, nil
		}
		name := args[0].ToString()
		value := args[1].ToString()
		h.headers.Set(name, value)
		return vm.Undefined, nil
	}))

	obj.SetOwnNonEnumerable("delete", vm.NewNativeFunction(1, false, "delete", func(args []vm.Value) (vm.Value, error) {
		if len(args) < 1 {
			return vm.Undefined, nil
		}
		name := args[0].ToString()
		h.headers.Del(name)
		return vm.Undefined, nil
	}))

	obj.SetOwnNonEnumerable("append", vm.NewNativeFunction(2, false, "append", func(args []vm.Value) (vm.Value, error) {
		if len(args) < 2 {
			return vm.Undefined, nil
		}
		name := args[0].ToString()
		value := args[1].ToString()
		h.headers.Add(name, value)
		return vm.Undefined, nil
	}))

	// entries() -> Iterator of [key, value] pairs
	entriesFn := vm.NewNativeFunction(0, false, "entries", func(args []vm.Value) (vm.Value, error) {
		// Snapshot entries into an array
		data := vm.NewArray()
		dataArr := data.AsArray()
		for name, values := range h.headers {
			for _, value := range values {
				pair := vm.NewArray()
				pairArr := pair.AsArray()
				pairArr.Append(vm.NewString(name))
				pairArr.Append(vm.NewString(value))
				dataArr.Append(pair)
			}
		}

		// Create iterator object
		it := vm.NewObject(vmInstance.ObjectPrototype).AsPlainObject()
		it.SetOwnNonEnumerable("__data__", data)
		it.SetOwnNonEnumerable("__index__", vm.IntegerValue(0))
		it.SetOwnNonEnumerable("next", vm.NewNativeFunction(0, false, "next", func(a []vm.Value) (vm.Value, error) {
			self := vmInstance.GetThis().AsPlainObject()
			dataVal, _ := self.GetOwn("__data__")
			idxVal, _ := self.GetOwn("__index__")
			dataArray := dataVal.AsArray()
			idx := int(idxVal.ToInteger())
			result := vm.NewObject(vm.Undefined).AsPlainObject()
			if idx >= dataArray.Length() {
				result.SetOwnNonEnumerable("value", vm.Undefined)
				result.SetOwnNonEnumerable("done", vm.True)
				return vm.NewValueFromPlainObject(result), nil
			}
			result.SetOwnNonEnumerable("value", dataArray.Get(idx))
			result.SetOwnNonEnumerable("done", vm.False)
			self.SetOwnNonEnumerable("__index__", vm.IntegerValue(int32(idx+1)))
			return vm.NewValueFromPlainObject(result), nil
		}))
		it.DefineOwnPropertyByKey(vm.NewSymbolKey(SymbolIterator), vm.NewNativeFunction(0, false, "[Symbol.iterator]", func(a []vm.Value) (vm.Value, error) {
			return vm.NewValueFromPlainObject(it), nil
		}), nil, nil, nil)
		return vm.NewValueFromPlainObject(it), nil
	})
	obj.SetOwnNonEnumerable("entries", entriesFn)

	// keys() -> Iterator of header names
	obj.SetOwnNonEnumerable("keys", vm.NewNativeFunction(0, false, "keys", func(args []vm.Value) (vm.Value, error) {
		// Snapshot keys into an array
		data := vm.NewArray()
		dataArr := data.AsArray()
		for name := range h.headers {
			dataArr.Append(vm.NewString(name))
		}

		// Create iterator object
		it := vm.NewObject(vmInstance.ObjectPrototype).AsPlainObject()
		it.SetOwnNonEnumerable("__data__", data)
		it.SetOwnNonEnumerable("__index__", vm.IntegerValue(0))
		it.SetOwnNonEnumerable("next", vm.NewNativeFunction(0, false, "next", func(a []vm.Value) (vm.Value, error) {
			self := vmInstance.GetThis().AsPlainObject()
			dataVal, _ := self.GetOwn("__data__")
			idxVal, _ := self.GetOwn("__index__")
			dataArray := dataVal.AsArray()
			idx := int(idxVal.ToInteger())
			result := vm.NewObject(vm.Undefined).AsPlainObject()
			if idx >= dataArray.Length() {
				result.SetOwnNonEnumerable("value", vm.Undefined)
				result.SetOwnNonEnumerable("done", vm.True)
				return vm.NewValueFromPlainObject(result), nil
			}
			result.SetOwnNonEnumerable("value", dataArray.Get(idx))
			result.SetOwnNonEnumerable("done", vm.False)
			self.SetOwnNonEnumerable("__index__", vm.IntegerValue(int32(idx+1)))
			return vm.NewValueFromPlainObject(result), nil
		}))
		it.DefineOwnPropertyByKey(vm.NewSymbolKey(SymbolIterator), vm.NewNativeFunction(0, false, "[Symbol.iterator]", func(a []vm.Value) (vm.Value, error) {
			return vm.NewValueFromPlainObject(it), nil
		}), nil, nil, nil)
		return vm.NewValueFromPlainObject(it), nil
	}))

	// values() -> Iterator of header values
	obj.SetOwnNonEnumerable("values", vm.NewNativeFunction(0, false, "values", func(args []vm.Value) (vm.Value, error) {
		// Snapshot values into an array
		data := vm.NewArray()
		dataArr := data.AsArray()
		for _, values := range h.headers {
			for _, value := range values {
				dataArr.Append(vm.NewString(value))
			}
		}

		// Create iterator object
		it := vm.NewObject(vmInstance.ObjectPrototype).AsPlainObject()
		it.SetOwnNonEnumerable("__data__", data)
		it.SetOwnNonEnumerable("__index__", vm.IntegerValue(0))
		it.SetOwnNonEnumerable("next", vm.NewNativeFunction(0, false, "next", func(a []vm.Value) (vm.Value, error) {
			self := vmInstance.GetThis().AsPlainObject()
			dataVal, _ := self.GetOwn("__data__")
			idxVal, _ := self.GetOwn("__index__")
			dataArray := dataVal.AsArray()
			idx := int(idxVal.ToInteger())
			result := vm.NewObject(vm.Undefined).AsPlainObject()
			if idx >= dataArray.Length() {
				result.SetOwnNonEnumerable("value", vm.Undefined)
				result.SetOwnNonEnumerable("done", vm.True)
				return vm.NewValueFromPlainObject(result), nil
			}
			result.SetOwnNonEnumerable("value", dataArray.Get(idx))
			result.SetOwnNonEnumerable("done", vm.False)
			self.SetOwnNonEnumerable("__index__", vm.IntegerValue(int32(idx+1)))
			return vm.NewValueFromPlainObject(result), nil
		}))
		it.DefineOwnPropertyByKey(vm.NewSymbolKey(SymbolIterator), vm.NewNativeFunction(0, false, "[Symbol.iterator]", func(a []vm.Value) (vm.Value, error) {
			return vm.NewValueFromPlainObject(it), nil
		}), nil, nil, nil)
		return vm.NewValueFromPlainObject(it), nil
	}))

	// forEach(callback) - executes callback for each header
	obj.SetOwnNonEnumerable("forEach", vm.NewNativeFunction(1, false, "forEach", func(args []vm.Value) (vm.Value, error) {
		if len(args) < 1 || !args[0].IsCallable() {
			return vm.Undefined, nil
		}
		callback := args[0]
		for name, values := range h.headers {
			for _, value := range values {
				_, _ = vmInstance.Call(callback, vm.Undefined, []vm.Value{
					vm.NewString(value),
					vm.NewString(name),
					vm.NewValueFromPlainObject(obj),
				})
			}
		}
		return vm.Undefined, nil
	}))

	// [Symbol.iterator] - calls entries() to return an iterator
	obj.DefineOwnPropertyByKey(vm.NewSymbolKey(SymbolIterator), vm.NewNativeFunction(0, false, "[Symbol.iterator]", func(args []vm.Value) (vm.Value, error) {
		// Call entries() to get the iterator
		return vmInstance.Call(entriesFn, vm.Undefined, []vm.Value{})
	}), nil, nil, nil)

	return vm.NewValueFromPlainObject(obj)
}

// createResponseObject creates a Response object for the VM with async methods
func createResponseObject(vmInstance *vm.VM, r *FetchResponse) vm.Value {
	obj := vm.NewObject(vmInstance.ObjectPrototype).AsPlainObject()

	// Read-only properties
	obj.SetOwn("ok", boolToValue(r.OK))
	obj.SetOwn("status", vm.NumberValue(float64(r.Status)))
	obj.SetOwn("statusText", vm.NewString(r.StatusText))
	obj.SetOwn("url", vm.NewString(r.URL))
	obj.SetOwn("headers", createHeadersObject(vmInstance, r.Headers))
	obj.SetOwn("bodyUsed", boolToValue(r.bodyUsed))
	obj.SetOwn("redirected", boolToValue(r.Redirected))
	obj.SetOwn("type", vm.NewString(r.Type))

	// text() -> Promise<string>
	obj.SetOwnNonEnumerable("text", vm.NewNativeFunction(0, false, "text", func(args []vm.Value) (vm.Value, error) {
		if r.bodyUsed {
			return vmInstance.NewRejectedPromise(vm.NewString("body already used")), nil
		}
		r.bodyUsed = true
		// Update bodyUsed property on the object
		obj.SetOwn("bodyUsed", vm.True)
		return vmInstance.NewResolvedPromise(vm.NewString(string(r.body))), nil
	}))

	// json() -> Promise<any>
	obj.SetOwnNonEnumerable("json", vm.NewNativeFunction(0, false, "json", func(args []vm.Value) (vm.Value, error) {
		if r.bodyUsed {
			return vmInstance.NewRejectedPromise(vm.NewString("body already used")), nil
		}
		r.bodyUsed = true
		obj.SetOwn("bodyUsed", vm.True)

		var result vm.Value
		if err := result.UnmarshalJSON(r.body); err != nil {
			return vmInstance.NewRejectedPromise(vm.NewString(err.Error())), nil
		}
		return vmInstance.NewResolvedPromise(result), nil
	}))

	// blob() -> Promise<Uint8Array>
	obj.SetOwnNonEnumerable("blob", vm.NewNativeFunction(0, false, "blob", func(args []vm.Value) (vm.Value, error) {
		if r.bodyUsed {
			return vmInstance.NewRejectedPromise(vm.NewString("body already used")), nil
		}
		r.bodyUsed = true
		obj.SetOwn("bodyUsed", vm.True)

		// Create Uint8Array from body bytes
		arrayBufferValue := vm.NewArrayBuffer(len(r.body))
		buffer := arrayBufferValue.AsArrayBuffer()
		copy(buffer.GetData(), r.body)
		uint8Array := vm.NewTypedArray(vm.TypedArrayUint8, buffer, 0, 0)
		return vmInstance.NewResolvedPromise(uint8Array), nil
	}))

	// arrayBuffer() -> Promise<ArrayBuffer>
	obj.SetOwnNonEnumerable("arrayBuffer", vm.NewNativeFunction(0, false, "arrayBuffer", func(args []vm.Value) (vm.Value, error) {
		if r.bodyUsed {
			return vmInstance.NewRejectedPromise(vm.NewString("body already used")), nil
		}
		r.bodyUsed = true
		obj.SetOwn("bodyUsed", vm.True)

		// Create ArrayBuffer from body bytes
		arrayBufferValue := vm.NewArrayBuffer(len(r.body))
		buffer := arrayBufferValue.AsArrayBuffer()
		copy(buffer.GetData(), r.body)
		return vmInstance.NewResolvedPromise(arrayBufferValue), nil
	}))

	// bytes() -> Promise<Uint8Array> (same as blob, but standard name)
	obj.SetOwnNonEnumerable("bytes", vm.NewNativeFunction(0, false, "bytes", func(args []vm.Value) (vm.Value, error) {
		if r.bodyUsed {
			return vmInstance.NewRejectedPromise(vm.NewString("body already used")), nil
		}
		r.bodyUsed = true
		obj.SetOwn("bodyUsed", vm.True)

		// Create Uint8Array from body bytes
		arrayBufferValue := vm.NewArrayBuffer(len(r.body))
		buffer := arrayBufferValue.AsArrayBuffer()
		copy(buffer.GetData(), r.body)
		uint8Array := vm.NewTypedArray(vm.TypedArrayUint8, buffer, 0, 0)
		return vmInstance.NewResolvedPromise(uint8Array), nil
	}))

	// clone() -> Response (creates a copy of the response)
	obj.SetOwnNonEnumerable("clone", vm.NewNativeFunction(0, false, "clone", func(args []vm.Value) (vm.Value, error) {
		if r.bodyUsed {
			return vm.Undefined, fmt.Errorf("TypeError: Response body is already used")
		}

		// Create a copy of the response with the same body
		clonedResponse := &FetchResponse{
			vm:         r.vm,
			OK:         r.OK,
			Status:     r.Status,
			StatusText: r.StatusText,
			URL:        r.URL,
			Headers:    &FetchHeaders{headers: r.Headers.headers.Clone()},
			body:       r.body, // Share the same body bytes (they're not modified)
			bodyUsed:   false,
			Redirected: r.Redirected,
			Type:       r.Type,
		}
		return createResponseObject(vmInstance, clonedResponse), nil
	}))

	return vm.NewValueFromPlainObject(obj)
}

// doFetchRequestWithContext performs the HTTP request with context support for cancellation
func doFetchRequestWithContext(ctx context.Context, vmInstance *vm.VM, url string, init vm.Value) (vm.Value, error) {
	// Default options
	method := "GET"
	headers := &FetchHeaders{headers: make(http.Header)}
	var body io.Reader
	var abortSignal *AbortSignal
	redirectMode := "follow" // "follow", "error", "manual"

	// Parse init options if provided
	if init.Type() != vm.TypeUndefined && init.Type() != vm.TypeNull {
		var initObj interface {
			GetOwn(string) (vm.Value, bool)
		}

		if init.Type() == vm.TypeObject {
			initObj = init.AsPlainObject()
		} else if init.Type() == vm.TypeDictObject {
			initObj = init.AsDictObject()
		}

		if initObj != nil {
			// Method
			if m, exists := initObj.GetOwn("method"); exists && m.Type() == vm.TypeString {
				method = strings.ToUpper(m.ToString())
			}

			// Headers
			if h, exists := initObj.GetOwn("headers"); exists && h.Type() != vm.TypeUndefined {
				if h.Type() == vm.TypeObject {
					hObj := h.AsPlainObject()
					for _, key := range hObj.OwnKeys() {
						if val, exists := hObj.GetOwn(key); exists {
							headers.headers.Set(key, val.ToString())
						}
					}
				} else if h.Type() == vm.TypeDictObject {
					hDictObj := h.AsDictObject()
					for _, key := range hDictObj.OwnKeys() {
						if val, exists := hDictObj.GetOwn(key); exists {
							headers.headers.Set(key, val.ToString())
						}
					}
				}
			}

			// Body
			if b, exists := initObj.GetOwn("body"); exists && b.Type() != vm.TypeUndefined {
				switch b.Type() {
				case vm.TypeString:
					body = strings.NewReader(b.ToString())
				default:
					// For objects, check if we should auto-stringify
					contentType := headers.headers.Get("Content-Type")
					if strings.Contains(strings.ToLower(contentType), "application/json") {
						// Auto-stringify objects for JSON content type
						jsonBytes, err := b.MarshalJSON()
						if err != nil {
							return vm.Undefined, fmt.Errorf("failed to serialize body to JSON: %w", err)
						}
						body = bytes.NewReader(jsonBytes)
					} else if b.Type() == vm.TypeObject || b.Type() == vm.TypeDictObject {
						// Default to JSON for objects
						jsonBytes, err := b.MarshalJSON()
						if err != nil {
							return vm.Undefined, fmt.Errorf("failed to serialize body to JSON: %w", err)
						}
						body = bytes.NewReader(jsonBytes)
					} else {
						body = strings.NewReader(b.ToString())
					}
				}
			}

			// Signal (AbortSignal)
			if s, exists := initObj.GetOwn("signal"); exists && s.Type() == vm.TypeObject {
				signalObj := s.AsPlainObject()
				// Check if signal is already aborted
				if aborted, exists := signalObj.GetOwn("aborted"); exists {
					if aborted.IsBoolean() && aborted.AsBoolean() {
						reason := vm.NewString("AbortError: signal is aborted without reason")
						if r, exists := signalObj.GetOwn("reason"); exists && r.Type() != vm.TypeUndefined {
							reason = r
						}
						return vm.Undefined, &AbortError{Message: reason.ToString()}
					}
				}
				// Store reference for potential future abort (would need more infrastructure)
				abortSignal = &AbortSignal{aborted: false}
				_ = abortSignal // Avoid unused variable warning
			}

			// Redirect mode
			if r, exists := initObj.GetOwn("redirect"); exists && r.Type() == vm.TypeString {
				redirectMode = r.ToString()
			}
		}
	}

	// Track if we were redirected
	redirected := false
	originalURL := url

	// Create HTTP client with reasonable timeout
	client := &http.Client{
		Timeout: 30 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) > 0 {
				redirected = true
			}
			switch redirectMode {
			case "error":
				return fmt.Errorf("fetch redirect not allowed")
			case "manual":
				return http.ErrUseLastResponse
			default: // "follow"
				if len(via) >= 20 {
					return fmt.Errorf("too many redirects")
				}
				return nil
			}
		},
	}

	// Create request with context for cancellation support
	req, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		return vm.Undefined, err
	}

	// Set headers
	req.Header = headers.headers

	// Perform request
	resp, err := client.Do(req)
	if err != nil {
		return vm.Undefined, err
	}
	defer resp.Body.Close()

	// Read response body
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return vm.Undefined, err
	}

	// Create response headers
	responseHeaders := &FetchHeaders{headers: resp.Header}

	// Determine response type
	responseType := "basic"
	respURL := resp.Request.URL.String()
	if respURL != originalURL {
		// Different origin after redirect
		responseType = "cors"
	}

	// Create Response object
	response := &FetchResponse{
		vm:         vmInstance,
		OK:         resp.StatusCode >= 200 && resp.StatusCode < 300,
		Status:     resp.StatusCode,
		StatusText: resp.Status,
		URL:        respURL,
		Headers:    responseHeaders,
		body:       bodyBytes,
		bodyUsed:   false,
		Redirected: redirected,
		Type:       responseType,
	}

	return createResponseObject(vmInstance, response), nil
}

// FetchRequest represents the Request object
type FetchRequest struct {
	vm             *vm.VM
	Method         string
	URL            string
	Headers        *FetchHeaders
	body           []byte
	bodyUsed       bool
	Cache          string
	Credentials    string
	Destination    string
	Integrity      string
	Mode           string
	Redirect       string
	Referrer       string
	ReferrerPolicy string
	Signal         vm.Value
}

// valueToBytes converts various value types to bytes
func valueToBytes(v vm.Value) []byte {
	switch v.Type() {
	case vm.TypeString:
		return []byte(v.ToString())
	case vm.TypeArrayBuffer:
		if buf := v.AsArrayBuffer(); buf != nil {
			data := make([]byte, len(buf.GetData()))
			copy(data, buf.GetData())
			return data
		}
	case vm.TypeTypedArray:
		if ta := v.AsTypedArray(); ta != nil {
			data := make([]byte, ta.GetLength())
			for i := 0; i < ta.GetLength(); i++ {
				data[i] = byte(ta.GetElement(i).ToFloat())
			}
			return data
		}
	case vm.TypeObject:
		// Try to serialize as JSON
		if jsonBytes, err := v.MarshalJSON(); err == nil {
			return jsonBytes
		}
		return []byte(v.ToString())
	default:
		return []byte(v.ToString())
	}
	return []byte{}
}

// parseHeaders parses a value into FetchHeaders
func parseHeaders(v vm.Value) *FetchHeaders {
	headers := &FetchHeaders{headers: make(http.Header)}
	if v.Type() == vm.TypeObject {
		obj := v.AsPlainObject()
		for _, key := range obj.OwnKeys() {
			if val, exists := obj.GetOwn(key); exists {
				headers.headers.Set(key, val.ToString())
			}
		}
	} else if v.Type() == vm.TypeDictObject {
		dictObj := v.AsDictObject()
		for _, key := range dictObj.OwnKeys() {
			if val, exists := dictObj.GetOwn(key); exists {
				headers.headers.Set(key, val.ToString())
			}
		}
	}
	return headers
}

// parseRequestInit parses RequestInit options from a PlainObject
func parseRequestInit(req *FetchRequest, initObj *vm.PlainObject) {
	if m, exists := initObj.GetOwn("method"); exists && m.Type() == vm.TypeString {
		req.Method = strings.ToUpper(m.ToString())
	}
	if h, exists := initObj.GetOwn("headers"); exists && h.Type() != vm.TypeUndefined {
		req.Headers = parseHeaders(h)
	}
	if b, exists := initObj.GetOwn("body"); exists && b.Type() != vm.TypeUndefined && b.Type() != vm.TypeNull {
		req.body = valueToBytes(b)
	}
	if c, exists := initObj.GetOwn("cache"); exists && c.Type() == vm.TypeString {
		req.Cache = c.ToString()
	}
	if c, exists := initObj.GetOwn("credentials"); exists && c.Type() == vm.TypeString {
		req.Credentials = c.ToString()
	}
	if m, exists := initObj.GetOwn("mode"); exists && m.Type() == vm.TypeString {
		req.Mode = m.ToString()
	}
	if r, exists := initObj.GetOwn("redirect"); exists && r.Type() == vm.TypeString {
		req.Redirect = r.ToString()
	}
	if r, exists := initObj.GetOwn("referrer"); exists && r.Type() == vm.TypeString {
		req.Referrer = r.ToString()
	}
	if r, exists := initObj.GetOwn("referrerPolicy"); exists && r.Type() == vm.TypeString {
		req.ReferrerPolicy = r.ToString()
	}
	if s, exists := initObj.GetOwn("signal"); exists && s.Type() != vm.TypeUndefined {
		req.Signal = s
	}
}

// parseRequestInitDict parses RequestInit options from a DictObject
func parseRequestInitDict(req *FetchRequest, initObj *vm.DictObject) {
	if m, exists := initObj.GetOwn("method"); exists && m.Type() == vm.TypeString {
		req.Method = strings.ToUpper(m.ToString())
	}
	if h, exists := initObj.GetOwn("headers"); exists && h.Type() != vm.TypeUndefined {
		req.Headers = parseHeaders(h)
	}
	if b, exists := initObj.GetOwn("body"); exists && b.Type() != vm.TypeUndefined && b.Type() != vm.TypeNull {
		req.body = valueToBytes(b)
	}
	if c, exists := initObj.GetOwn("cache"); exists && c.Type() == vm.TypeString {
		req.Cache = c.ToString()
	}
	if c, exists := initObj.GetOwn("credentials"); exists && c.Type() == vm.TypeString {
		req.Credentials = c.ToString()
	}
	if m, exists := initObj.GetOwn("mode"); exists && m.Type() == vm.TypeString {
		req.Mode = m.ToString()
	}
	if r, exists := initObj.GetOwn("redirect"); exists && r.Type() == vm.TypeString {
		req.Redirect = r.ToString()
	}
	if r, exists := initObj.GetOwn("referrer"); exists && r.Type() == vm.TypeString {
		req.Referrer = r.ToString()
	}
	if r, exists := initObj.GetOwn("referrerPolicy"); exists && r.Type() == vm.TypeString {
		req.ReferrerPolicy = r.ToString()
	}
	if s, exists := initObj.GetOwn("signal"); exists && s.Type() != vm.TypeUndefined {
		req.Signal = s
	}
}

// createRequestObject creates a Request object for the VM
func createRequestObject(vmInstance *vm.VM, req *FetchRequest, _ *vm.PlainObject) vm.Value {
	obj := vm.NewObject(vmInstance.ObjectPrototype).AsPlainObject()

	// Read-only properties
	obj.SetOwn("method", vm.NewString(req.Method))
	obj.SetOwn("url", vm.NewString(req.URL))
	obj.SetOwn("headers", createHeadersObject(vmInstance, req.Headers))
	obj.SetOwn("body", vm.Null) // Body is null for most requests
	obj.SetOwn("bodyUsed", boolToValue(req.bodyUsed))
	obj.SetOwn("cache", vm.NewString(req.Cache))
	obj.SetOwn("credentials", vm.NewString(req.Credentials))
	obj.SetOwn("destination", vm.NewString(req.Destination))
	obj.SetOwn("integrity", vm.NewString(req.Integrity))
	obj.SetOwn("mode", vm.NewString(req.Mode))
	obj.SetOwn("redirect", vm.NewString(req.Redirect))
	obj.SetOwn("referrer", vm.NewString(req.Referrer))
	obj.SetOwn("referrerPolicy", vm.NewString(req.ReferrerPolicy))
	obj.SetOwn("signal", req.Signal)

	// clone() -> Request
	obj.SetOwnNonEnumerable("clone", vm.NewNativeFunction(0, false, "clone", func(args []vm.Value) (vm.Value, error) {
		if req.bodyUsed {
			return vm.Undefined, fmt.Errorf("TypeError: Request body is already used")
		}

		clonedReq := &FetchRequest{
			vm:             vmInstance,
			Method:         req.Method,
			URL:            req.URL,
			Headers:        &FetchHeaders{headers: req.Headers.headers.Clone()},
			body:           req.body,
			bodyUsed:       false,
			Cache:          req.Cache,
			Credentials:    req.Credentials,
			Destination:    req.Destination,
			Integrity:      req.Integrity,
			Mode:           req.Mode,
			Redirect:       req.Redirect,
			Referrer:       req.Referrer,
			ReferrerPolicy: req.ReferrerPolicy,
			Signal:         req.Signal,
		}
		return createRequestObject(vmInstance, clonedReq, nil), nil
	}))

	// arrayBuffer() -> Promise<ArrayBuffer>
	obj.SetOwnNonEnumerable("arrayBuffer", vm.NewNativeFunction(0, false, "arrayBuffer", func(args []vm.Value) (vm.Value, error) {
		if req.bodyUsed {
			return vmInstance.NewRejectedPromise(vm.NewString("body already used")), nil
		}
		req.bodyUsed = true
		obj.SetOwn("bodyUsed", vm.True)

		if req.body == nil {
			return vmInstance.NewResolvedPromise(vm.NewArrayBuffer(0)), nil
		}

		arrayBuffer := vm.NewArrayBuffer(len(req.body))
		buf := arrayBuffer.AsArrayBuffer()
		copy(buf.GetData(), req.body)
		return vmInstance.NewResolvedPromise(arrayBuffer), nil
	}))

	// blob() -> Promise<Blob>
	obj.SetOwnNonEnumerable("blob", vm.NewNativeFunction(0, false, "blob", func(args []vm.Value) (vm.Value, error) {
		if req.bodyUsed {
			return vmInstance.NewRejectedPromise(vm.NewString("body already used")), nil
		}
		req.bodyUsed = true
		obj.SetOwn("bodyUsed", vm.True)

		if req.body == nil {
			return vmInstance.NewResolvedPromise(vm.NewArrayBuffer(0)), nil
		}

		arrayBuffer := vm.NewArrayBuffer(len(req.body))
		buf := arrayBuffer.AsArrayBuffer()
		copy(buf.GetData(), req.body)
		uint8Array := vm.NewTypedArray(vm.TypedArrayUint8, buf, 0, 0)
		return vmInstance.NewResolvedPromise(uint8Array), nil
	}))

	// json() -> Promise<any>
	obj.SetOwnNonEnumerable("json", vm.NewNativeFunction(0, false, "json", func(args []vm.Value) (vm.Value, error) {
		if req.bodyUsed {
			return vmInstance.NewRejectedPromise(vm.NewString("body already used")), nil
		}
		req.bodyUsed = true
		obj.SetOwn("bodyUsed", vm.True)

		if req.body == nil {
			return vmInstance.NewRejectedPromise(vm.NewString("Unexpected end of JSON input")), nil
		}

		parsed, err := parseJSONToValue(string(req.body))
		if err != nil {
			return vmInstance.NewRejectedPromise(vm.NewString(err.Error())), nil
		}
		return vmInstance.NewResolvedPromise(parsed), nil
	}))

	// text() -> Promise<string>
	obj.SetOwnNonEnumerable("text", vm.NewNativeFunction(0, false, "text", func(args []vm.Value) (vm.Value, error) {
		if req.bodyUsed {
			return vmInstance.NewRejectedPromise(vm.NewString("body already used")), nil
		}
		req.bodyUsed = true
		obj.SetOwn("bodyUsed", vm.True)

		if req.body == nil {
			return vmInstance.NewResolvedPromise(vm.NewString("")), nil
		}
		return vmInstance.NewResolvedPromise(vm.NewString(string(req.body))), nil
	}))

	// formData() -> Promise<FormData> (stub - would need FormData parsing)
	obj.SetOwnNonEnumerable("formData", vm.NewNativeFunction(0, false, "formData", func(args []vm.Value) (vm.Value, error) {
		return vmInstance.NewRejectedPromise(vm.NewString("formData() parsing not yet implemented")), nil
	}))

	return vm.NewValueFromPlainObject(obj)
}
