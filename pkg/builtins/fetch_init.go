package builtins

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/nooga/paserati/pkg/runtime"
	"github.com/nooga/paserati/pkg/types"
	"github.com/nooga/paserati/pkg/vm"
)

// Priority constant for fetch
const PriorityFetch = 200 // After most builtins

// FetchResponseHeaderTimeout bounds how long fetch() waits for response
// headers to arrive before failing with a network error; it does not bound
// reading the body afterward (see doFetchRequestWithContext). Defaults to
// 30s. A host embedding this engine may set this (before any fetch() call -
// it is read fresh per-request but not synchronized against concurrent
// in-flight requests) to match its own configurable timeout, or to 0 to
// disable the timeout entirely (#290).
var FetchResponseHeaderTimeout = 30 * time.Second

// FetchProxy selects the proxy to use for a given outbound fetch() request,
// in the same shape as http.Transport.Proxy. Defaults to
// http.ProxyFromEnvironment so the standard HTTP_PROXY/HTTPS_PROXY/NO_PROXY
// environment variables are respected without any host-side configuration -
// http.Transport's own zero value leaves this nil, which silently makes
// direct connections instead of erroring, so this default matters. A host
// may override it (e.g. to force a specific proxy, or nil to disable
// proxying) before any fetch() call (#290).
var FetchProxy func(*http.Request) (*url.URL, error) = http.ProxyFromEnvironment

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
		WithProperty("blob", types.NewSimpleFunction([]types.Type{}, types.Any)).        // Returns Promise<Blob>
		WithProperty("arrayBuffer", types.NewSimpleFunction([]types.Type{}, types.Any)). // Returns Promise<ArrayBuffer>
		WithProperty("bytes", types.NewSimpleFunction([]types.Type{}, types.Any)).       // Returns Promise<Uint8Array>
		WithProperty("formData", types.NewSimpleFunction([]types.Type{}, types.Any)).    // Returns Promise<FormData>
		WithProperty("clone", types.NewSimpleFunction([]types.Type{}, types.Any))        // Returns Response

	// ResponseInit type
	responseInitType := types.NewObjectType().
		WithOptionalProperty("status", types.Number).
		WithOptionalProperty("statusText", types.String).
		WithOptionalProperty("headers", types.NewUnionType(headersType, types.NewObjectType()))

	// Response constructor type
	responseConstructorType := types.NewObjectType().
		WithSimpleCallSignature([]types.Type{}, responseType).                            // Response()
		WithSimpleCallSignature([]types.Type{types.Any}, responseType).                   // Response(body)
		WithSimpleCallSignature([]types.Type{types.Any, responseInitType}, responseType). // Response(body, init)
		WithProperty("prototype", responseType).
		WithProperty("error", types.NewSimpleFunction([]types.Type{}, responseType)).                              // Response.error()
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
		WithOptionalProperty("signal", types.Any).         // AbortSignal
		WithOptionalProperty("redirect", types.String).    // "follow" | "error" | "manual"
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
		WithProperty("signal", types.Any).                                               // AbortSignal
		WithProperty("clone", types.NewSimpleFunction([]types.Type{}, types.Any)).       // Returns Request
		WithProperty("arrayBuffer", types.NewSimpleFunction([]types.Type{}, types.Any)). // Returns Promise<ArrayBuffer>
		WithProperty("blob", types.NewSimpleFunction([]types.Type{}, types.Any)).        // Returns Promise<Blob>
		WithProperty("formData", types.NewSimpleFunction([]types.Type{}, types.Any)).    // Returns Promise<FormData>
		WithProperty("json", types.NewSimpleFunction([]types.Type{}, types.Any)).        // Returns Promise<any>
		WithProperty("text", types.NewSimpleFunction([]types.Type{}, types.Any))         // Returns Promise<string>

	// Request constructor type
	requestConstructorType := types.NewObjectType().
		WithSimpleCallSignature([]types.Type{types.String}, requestType).                  // Request(url)
		WithSimpleCallSignature([]types.Type{types.String, requestInitType}, requestType). // Request(url, init)
		WithSimpleCallSignature([]types.Type{requestType}, requestType).                   // Request(request)
		WithSimpleCallSignature([]types.Type{requestType, requestInitType}, requestType).  // Request(request, init)
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
			mergeHeadersFrom(headers, args[0])
		}

		return createHeadersObject(vmInstance, headers), nil
	}

	// Use NewConstructorWithProps to make it callable with 'new'
	headersConstructor := vm.NewConstructorWithProps(1, false, "Headers", headersConstructorFn)
	if headersConstructor.Type() == vm.TypeNativeFunctionWithProps {
		ctorProps := headersConstructor.AsNativeFunctionWithProps()
		ctorProps.Properties.DefineFixedProperty("prototype", vm.NewValueFromPlainObject(headersProto))
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

		bodyState, bodyStream := newImmediateFetchBody(vmInstance, bodyBytes)
		response := &FetchResponse{
			vm:         vmInstance,
			OK:         status >= 200 && status < 300,
			Status:     status,
			StatusText: statusText,
			URL:        "",
			Headers:    headers,
			bodyState:  bodyState,
			bodyStream: bodyStream,
			bodyUsed:   false,
			Redirected: false,
			Type:       "default",
		}

		return createResponseObject(vmInstance, response), nil
	}

	responseConstructor := vm.NewConstructorWithProps(2, false, "Response", responseConstructorFn)
	if responseConstructor.Type() == vm.TypeNativeFunctionWithProps {
		ctorProps := responseConstructor.AsNativeFunctionWithProps()
		ctorProps.Properties.DefineFixedProperty("prototype", vm.NewValueFromPlainObject(responseProto))

		// Response.error() - returns an error response
		ctorProps.Properties.SetOwnNonEnumerable("error", vm.NewNativeFunction(0, false, "error", func(args []vm.Value) (vm.Value, error) {
			bodyState, bodyStream := newImmediateFetchBody(vmInstance, nil)
			response := &FetchResponse{
				vm:         vmInstance,
				OK:         false,
				Status:     0,
				StatusText: "",
				URL:        "",
				Headers:    &FetchHeaders{headers: make(http.Header)},
				bodyState:  bodyState,
				bodyStream: bodyStream,
				bodyUsed:   false,
				Redirected: false,
				Type:       "error",
			}
			return createResponseObject(vmInstance, response), nil
		}))

		// Response.redirect(url, status?) - returns a redirect response
		ctorProps.Properties.SetOwnNonEnumerable("redirect", vm.NewNativeFunction(2, false, "redirect", func(args []vm.Value) (vm.Value, error) {
			if len(args) < 1 {
				return vm.Undefined, vmInstance.NewTypeError("Response.redirect requires a URL")
			}
			url := args[0].ToString()
			status := 302 // Default redirect status
			if len(args) > 1 && args[1].IsNumber() {
				status = int(args[1].ToFloat())
			}
			// Validate redirect status
			if status != 301 && status != 302 && status != 303 && status != 307 && status != 308 {
				return vm.Undefined, vmInstance.NewRangeError("Invalid redirect status code")
			}
			headers := &FetchHeaders{headers: make(http.Header)}
			headers.headers.Set("Location", url)
			bodyState, bodyStream := newImmediateFetchBody(vmInstance, nil)
			response := &FetchResponse{
				vm:         vmInstance,
				OK:         false,
				Status:     status,
				StatusText: http.StatusText(status),
				URL:        "",
				Headers:    headers,
				bodyState:  bodyState,
				bodyStream: bodyStream,
				bodyUsed:   false,
				Redirected: false,
				Type:       "default",
			}
			return createResponseObject(vmInstance, response), nil
		}))

		// Response.json(data, init?) - returns a response with JSON body
		ctorProps.Properties.SetOwnNonEnumerable("json", vm.NewNativeFunction(2, false, "json", func(args []vm.Value) (vm.Value, error) {
			if len(args) < 1 {
				return vm.Undefined, vmInstance.NewTypeError("Response.json requires data")
			}
			jsonBytes, err := args[0].MarshalJSON()
			if err != nil {
				return vm.Undefined, vmInstance.NewTypeError("Failed to serialize data to JSON")
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

			bodyState, bodyStream := newImmediateFetchBody(vmInstance, jsonBytes)
			response := &FetchResponse{
				vm:         vmInstance,
				OK:         status >= 200 && status < 300,
				Status:     status,
				StatusText: statusText,
				URL:        "",
				Headers:    headers,
				bodyState:  bodyState,
				bodyStream: bodyStream,
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
			return vm.Undefined, vmInstance.NewTypeError("Request constructor requires at least 1 argument")
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
	if requestConstructor.Type() == vm.TypeNativeFunctionWithProps {
		ctorProps := requestConstructor.AsNativeFunctionWithProps()
		ctorProps.Properties.DefineFixedProperty("prototype", vm.NewValueFromPlainObject(requestProto))
	}

	requestProto.SetOwnNonEnumerable("constructor", requestConstructor)

	if err := ctx.DefineGlobal("Request", requestConstructor); err != nil {
		return err
	}

	// Create fetch function - truly async via goroutines
	fetchFn := vm.NewNativeFunction(2, false, "fetch", func(args []vm.Value) (vm.Value, error) {
		if len(args) < 1 {
			return vm.Undefined, vmInstance.NewTypeError("fetch requires at least 1 argument")
		}

		var explicitInit vm.Value = vm.Undefined
		if len(args) > 1 {
			explicitInit = args[1]
		}

		// fetch(input, init) is spec'd as effectively `new Request(input,
		// init)` followed by sending that request - so when input is itself
		// a Request instance (recovered via the internal slot
		// createRequestObject stashes on it, the same pattern
		// mergeHeadersFrom uses for Headers), pull url/method/headers/body/
		// signal/redirect off of it as defaults, then let an explicit init
		// argument's own properties override them. Without this, args[0]
		// was stringified unconditionally below (ToString() on a Request
		// object produces "[object Object]", which then fails as an
		// unsupported protocol) and any options carried on the Request
		// (method, body, signal, ...) were silently dropped.
		url := args[0].ToString()
		init := explicitInit
		if args[0].Type() == vm.TypeObject {
			if req, ok := args[0].AsPlainObject().InternalSlots().(*FetchRequest); ok && req != nil {
				url = req.URL

				merged := vm.NewObject(vmInstance.ObjectPrototype).AsPlainObject()
				merged.SetOwn("method", vm.NewString(req.Method))
				merged.SetOwn("headers", createHeadersObject(vmInstance, &FetchHeaders{headers: req.Headers.headers.Clone()}))
				if req.body != nil {
					merged.SetOwn("body", vm.NewString(string(req.body)))
				}
				merged.SetOwn("redirect", vm.NewString(req.Redirect))
				if req.Signal.Type() != vm.TypeUndefined {
					merged.SetOwn("signal", req.Signal)
				}

				// Let an explicit init argument override any of the above,
				// per fetch(request, init) / new Request(request, init)
				// semantics.
				if explicitInit.Type() == vm.TypeObject {
					explicitObj := explicitInit.AsPlainObject()
					for _, key := range explicitObj.OwnKeys() {
						if val, exists := explicitObj.GetOwn(key); exists {
							merged.SetOwn(key, val)
						}
					}
				} else if explicitInit.Type() == vm.TypeDictObject {
					explicitObj := explicitInit.AsDictObject()
					for _, key := range explicitObj.OwnKeys() {
						if val, exists := explicitObj.GetOwn(key); exists {
							merged.SetOwn(key, val)
						}
					}
				}

				init = vm.NewValueFromPlainObject(merged)
			}
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
							reason := "signal is aborted without reason"
							if r, exists := signalObj.GetOwn("reason"); exists && r.Type() != vm.TypeUndefined {
								reason = reasonToMessage(r)
							}
							promise := vmInstance.NewPendingPromise()
							promiseObj := promise.AsPromise()
							vmInstance.RejectPromise(promiseObj, newAbortErrorValue(vmInstance, reason))
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
		var signalValue vm.Value
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
					signalValue = s
				}
			}
		}

		// If we have a signal, cancel the request's context the moment it
		// fires "abort" - registered through the signal's own
		// addEventListener, the same path user code would use, right here
		// on this native call's own goroutine (fetch() itself always runs
		// on the VM's goroutine; only the request below is backgrounded).
		// abort() dispatches synchronously and always from that same VM
		// goroutine too - directly from a script's controller.abort() call,
		// or from a due timer's RunDueTimers callback (AbortSignal.timeout(),
		// #374) - so this callback firing, and the SetOwn writes it
		// triggers, never race this function's own background request
		// goroutine. That's why this isn't a 10ms-poll of signalObj's
		// properties from that goroutine like it used to be: GetOwn/SetOwn
		// aren't synchronized for concurrent access, and polling from a
		// second goroutine while abort() writes from this one was a real
		// (if narrow) data race - `go test -race` catches it as soon as an
		// abort can happen without user code on the VM goroutine ever
		// observing it first, which is exactly what an async timer fire
		// looks like.
		if signalObj != nil {
			if addListenerFn, exists := signalObj.GetOwn("addEventListener"); exists && addListenerFn.IsCallable() {
				onAbort := vm.NewNativeFunction(1, false, "", func(_ []vm.Value) (vm.Value, error) {
					cancel() // context.CancelFunc is idempotent; abort() itself only ever fires once anyway.
					return vm.Undefined, nil
				})
				_, _ = vmInstance.Call(addListenerFn, signalValue, []vm.Value{vm.NewString("abort"), onAbort})
			}
		}

		// Perform HTTP request asynchronously in a goroutine. On a response
		// that streams (the common case, #205), cancel/rt outlive this
		// goroutine and are discharged later by the body-drain goroutine
		// doFetchRequestWithContext starts instead, *after* doFetchRequestWithContext
		// has already resolved promiseObj itself (#238); on an early-return
		// error, *this* goroutine owns them, and must discharge them only
		// after settling the promise below (#213 - see the doc comment on
		// doFetchRequestWithContext for why the order matters: cancel()/
		// rt.EndExternalOp() wake the driver's event-loop drain, and doing
		// that before the rejection is scheduled can make the drain exit
		// with the promise settled but its continuation never resumed).
		go func() {
			err := doFetchRequestWithContext(ctx, cancel, rt, vmInstance, promiseObj, url, init)

			if err != nil {
				// Classify by the error type doFetchRequestWithContext
				// returned, *not* ctx.Err(): a naive post-hoc ctx.Err() ==
				// context.Canceled check can't tell a real abort from a
				// plain network error once cleanup (which cancels ctx) has
				// run - and per the above, cleanup must run before we reach
				// this line for the case that actually reached this code -
				// so this checks the error's own type instead (#213: this
				// branch used to be checked via ctx.Err(), which was always
				// true here, so every non-abort failure falsely rejected as
				// AbortError instead of the real error).
				if _, ok := errors.AsType[*AbortError](err); ok {
					reason := "The operation was aborted"
					if signalObj != nil {
						if r, exists := signalObj.GetOwn("reason"); exists && r.Type() != vm.TypeUndefined {
							reason = reasonToMessage(r)
						}
					}
					vmInstance.RejectPromise(promiseObj, newAbortErrorValue(vmInstance, reason))
				} else {
					vmInstance.RejectPromise(promiseObj, newTypeErrorValue(vmInstance, err.Error()))
				}
				// Settled above; safe to wake the driver's drain loop now.
				cancel()
				rt.EndExternalOp()
			}
			// Success: doFetchRequestWithContext already resolved promiseObj
			// and handed cancel/rt off to its own body-drain goroutine.
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
	vm         *vm.VM
	OK         bool
	Status     int
	StatusText string
	URL        string
	Headers    *FetchHeaders
	bodyState  *fetchBody // accumulates/holds the body bytes; see fetchBody
	bodyStream vm.Value   // the ReadableStream exposed as Response.body (#205)
	bodyUsed   bool
	Redirected bool   // Whether this response is the result of a redirect
	Type       string // Response type: "basic", "cors", "default", "error", "opaque", "opaqueredirect"
}

// fetchBody holds a Response body's bytes as they become known, whether that
// happens all at once (a synchronously constructed Response, or a static
// factory like Response.json()) or incrementally as a real network response
// streams in. doneCh closes exactly once, when the bytes are final (clean
// EOF or a read error) - text()/json()/blob()/arrayBuffer()/bytes() block on
// it (off the VM goroutine, see createResponseObject) rather than assuming
// the body is already fully buffered by the time they're called.
type fetchBody struct {
	mu     sync.Mutex
	buf    bytes.Buffer
	err    error
	doneCh chan struct{}
}

func newFetchBody() *fetchBody {
	return &fetchBody{doneCh: make(chan struct{})}
}

// appendChunk records a chunk that has already been enqueued on the
// ReadableStream side. Safe to call from any goroutine.
func (b *fetchBody) appendChunk(chunk []byte) {
	b.mu.Lock()
	b.buf.Write(chunk)
	b.mu.Unlock()
}

// finish marks the body as final, unblocking every wait(). err is nil for a
// clean end of stream. Must be called exactly once.
func (b *fetchBody) finish(err error) {
	b.mu.Lock()
	b.err = err
	b.mu.Unlock()
	close(b.doneCh)
}

// wait blocks until the body is final and returns its accumulated bytes (or
// the read error, if any). Only ever call this from a throwaway goroutine,
// never from the VM's own execution goroutine - a still-streaming body can
// take arbitrarily long (or, on an abandoned connection, forever).
func (b *fetchBody) wait() ([]byte, error) {
	<-b.doneCh
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]byte(nil), b.buf.Bytes()...), b.err
}

// isDone reports whether the body is already final, without blocking.
func (b *fetchBody) isDone() bool {
	select {
	case <-b.doneCh:
		return true
	default:
		return false
	}
}

// newImmediateFetchBody wraps already-fully-known bytes (a Response built by
// `new Response(...)`, `Response.json()`/`.error()`/`.redirect()`, or
// `.clone()`) as a fetchBody plus a matching ReadableStream containing
// exactly those bytes, then closed. This keeps `.body` a real ReadableStream
// (per spec) for every Response, not only ones a real fetch() produced,
// while text()/json()/etc. resolve exactly as they did before #205.
func newImmediateFetchBody(vmInstance *vm.VM, data []byte) (*fetchBody, vm.Value) {
	fb := newFetchBody()
	fb.buf.Write(data)
	fb.finish(nil)

	streamVal, controller := NewHostFedReadableStream(vmInstance)
	if len(data) > 0 {
		controller.EnqueueBytes(data)
	}
	controller.Close()
	return fb, streamVal
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

	// Brand the object with its backing FetchHeaders so code that reads a
	// Headers-shaped value (fetch()'s request builder, parseHeaders, the
	// Headers constructor itself) can recover the actual header data. Every
	// accessor below is installed non-enumerable, so OwnKeys() on this
	// object is always empty - reading headers via OwnKeys() alone silently
	// drops everything set through .set()/.append()/new Headers(...) (#237).
	obj.SetInternalSlots(h)

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
	obj.SetOwn("body", r.bodyStream)

	// drainBody waits for the body's full bytes - off the VM goroutine, if a
	// real fetch() response is still streaming in (see fetchBody) - and
	// settles a promise with transform's result. Shared by text()/json()/
	// blob()/arrayBuffer()/bytes(); they only differ in transform.
	drainBody := func(transform func([]byte) (vm.Value, error)) vm.Value {
		if r.bodyState.isDone() {
			data, err := r.bodyState.wait() // already final; does not block
			if err != nil {
				return vmInstance.NewRejectedPromise(vm.NewString(err.Error()))
			}
			result, err := transform(data)
			if err != nil {
				return vmInstance.NewRejectedPromise(vm.NewString(err.Error()))
			}
			return vmInstance.NewResolvedPromise(result)
		}
		promise := vmInstance.NewPendingPromise()
		promiseObj := promise.AsPromise()
		rt := vmInstance.GetAsyncRuntime()
		rt.BeginExternalOp()
		go func() {
			defer rt.EndExternalOp()
			data, err := r.bodyState.wait()
			if err != nil {
				vmInstance.RejectPromise(promiseObj, vm.NewString(err.Error()))
				return
			}
			result, err := transform(data)
			if err != nil {
				vmInstance.RejectPromise(promiseObj, vm.NewString(err.Error()))
				return
			}
			vmInstance.ResolvePromise(promiseObj, result)
		}()
		return promise
	}

	// text() -> Promise<string>
	obj.SetOwnNonEnumerable("text", vm.NewNativeFunction(0, false, "text", func(args []vm.Value) (vm.Value, error) {
		if r.bodyUsed {
			return vmInstance.NewRejectedPromise(vm.NewString("body already used")), nil
		}
		r.bodyUsed = true
		// Update bodyUsed property on the object
		obj.SetOwn("bodyUsed", vm.True)
		return drainBody(func(data []byte) (vm.Value, error) {
			return vm.NewString(string(data)), nil
		}), nil
	}))

	// json() -> Promise<any>
	obj.SetOwnNonEnumerable("json", vm.NewNativeFunction(0, false, "json", func(args []vm.Value) (vm.Value, error) {
		if r.bodyUsed {
			return vmInstance.NewRejectedPromise(vm.NewString("body already used")), nil
		}
		r.bodyUsed = true
		obj.SetOwn("bodyUsed", vm.True)
		return drainBody(func(data []byte) (vm.Value, error) {
			var result vm.Value
			if err := result.UnmarshalJSON(data); err != nil {
				return vm.Undefined, err
			}
			return result, nil
		}), nil
	}))

	// blob() -> Promise<Blob>
	obj.SetOwnNonEnumerable("blob", vm.NewNativeFunction(0, false, "blob", func(args []vm.Value) (vm.Value, error) {
		if r.bodyUsed {
			return vmInstance.NewRejectedPromise(vm.NewString("body already used")), nil
		}
		r.bodyUsed = true
		obj.SetOwn("bodyUsed", vm.True)
		mimeType := blobTypeFromContentType(r.Headers.headers.Get("Content-Type"))
		return drainBody(func(data []byte) (vm.Value, error) {
			return NewBlobValue(vmInstance, data, mimeType), nil
		}), nil
	}))

	// arrayBuffer() -> Promise<ArrayBuffer>
	obj.SetOwnNonEnumerable("arrayBuffer", vm.NewNativeFunction(0, false, "arrayBuffer", func(args []vm.Value) (vm.Value, error) {
		if r.bodyUsed {
			return vmInstance.NewRejectedPromise(vm.NewString("body already used")), nil
		}
		r.bodyUsed = true
		obj.SetOwn("bodyUsed", vm.True)
		return drainBody(func(data []byte) (vm.Value, error) {
			arrayBufferValue := vm.NewArrayBuffer(len(data))
			copy(arrayBufferValue.AsArrayBuffer().GetData(), data)
			return arrayBufferValue, nil
		}), nil
	}))

	// formData() -> Promise<FormData> (#397)
	obj.SetOwnNonEnumerable("formData", vm.NewNativeFunction(0, false, "formData", func(args []vm.Value) (vm.Value, error) {
		if r.bodyUsed {
			return vmInstance.NewRejectedPromise(vm.NewString("body already used")), nil
		}
		r.bodyUsed = true
		obj.SetOwn("bodyUsed", vm.True)
		contentType := r.Headers.headers.Get("Content-Type")
		return drainBody(func(data []byte) (vm.Value, error) {
			return parseMultipartFormData(vmInstance, contentType, data)
		}), nil
	}))

	// bytes() -> Promise<Uint8Array> (same as blob, but standard name)
	obj.SetOwnNonEnumerable("bytes", vm.NewNativeFunction(0, false, "bytes", func(args []vm.Value) (vm.Value, error) {
		if r.bodyUsed {
			return vmInstance.NewRejectedPromise(vm.NewString("body already used")), nil
		}
		r.bodyUsed = true
		obj.SetOwn("bodyUsed", vm.True)
		return drainBody(func(data []byte) (vm.Value, error) {
			return bytesToValue(data), nil
		}), nil
	}))

	// clone() -> Response (creates a copy of the response)
	obj.SetOwnNonEnumerable("clone", vm.NewNativeFunction(0, false, "clone", func(args []vm.Value) (vm.Value, error) {
		if r.bodyUsed {
			return vm.Undefined, vmInstance.NewTypeError("Response body is already used")
		}
		if !r.bodyState.isDone() {
			// No tee() on the underlying ReadableStream primitive (#205 is
			// deliberately scoped without one) - cloning a real fetch()
			// response mid-stream has nothing safe to copy yet.
			return vm.Undefined, vmInstance.NewTypeError("Response.clone(): cannot clone a response body that is still streaming")
		}
		data, _ := r.bodyState.wait() // isDone() above; does not block
		clonedBodyState, clonedBodyStream := newImmediateFetchBody(vmInstance, data)

		// Create a copy of the response with the same body bytes
		clonedResponse := &FetchResponse{
			vm:         r.vm,
			OK:         r.OK,
			Status:     r.Status,
			StatusText: r.StatusText,
			URL:        r.URL,
			Headers:    &FetchHeaders{headers: r.Headers.headers.Clone()},
			bodyState:  clonedBodyState,
			bodyStream: clonedBodyStream,
			bodyUsed:   false,
			Redirected: r.Redirected,
			Type:       r.Type,
		}
		return createResponseObject(vmInstance, clonedResponse), nil
	}))

	return vm.NewValueFromPlainObject(obj)
}

// newErrorValueWithPrototype builds an Error-shaped instance directly as
// data - NOT by calling the Error/TypeError constructor (vm.Call). fetch()'s
// completion runs on its own goroutine, off the VM's own goroutine, and
// vm.Call mutates shared VM fields (currentThis/currentNewTarget) around
// the native call with no synchronization against the main interpreter
// loop; building the instance by hand instead only touches data freshly
// allocated for this call, which is safe from any goroutine.
func newErrorValueWithPrototype(proto vm.Value, name, message string) vm.Value {
	errVal := vm.NewObject(proto)
	obj := errVal.AsPlainObject()
	obj.SetOwnNonEnumerable("name", vm.NewString(name))
	obj.SetOwnNonEnumerable("message", vm.NewString(message))
	return errVal
}

// reasonToMessage renders an AbortSignal's `reason` as a message string for
// fetch()'s own synthesized AbortError. Every call site here runs off the
// main VM goroutine (see newErrorValueWithPrototype above for why), which
// rules out invoking a JS-level toString() to stringify an arbitrary
// object reason - so an Error-shaped reason (an own "name"/"message" pair
// of *data* properties, exactly what newErrorValueWithPrototype itself
// builds - AbortController's default reason, AbortSignal.timeout()'s
// TimeoutError, etc.) is rendered the way Error.prototype.toString would
// ("name: message"), read directly via GetOwn rather than a getter call.
// A string reason is used as-is; anything else falls back to
// Value.ToString(), which degrades to "[object Object]" for an arbitrary
// non-Error object reason - the same unhelpful-but-harmless default a bare
// Error()'s toString would give for a message that isn't a string.
func reasonToMessage(reason vm.Value) string {
	if reason.Type() == vm.TypeObject {
		obj := reason.AsPlainObject()
		nameVal, hasName := obj.GetOwn("name")
		msgVal, hasMsg := obj.GetOwn("message")
		if hasName || hasMsg {
			name := "Error"
			if hasName && nameVal.Type() == vm.TypeString {
				name = nameVal.ToString()
			}
			if hasMsg && msgVal.Type() == vm.TypeString && msgVal.ToString() != "" {
				return name + ": " + msgVal.ToString()
			}
			return name
		}
	}
	return reason.ToString()
}

// newAbortErrorValue builds a real Error instance (name "AbortError") so a
// fetch() abort rejects with an actual Error object rather than a bare
// string (#214) - there is no DOMException in this runtime to construct
// instead.
func newAbortErrorValue(vmInstance *vm.VM, message string) vm.Value {
	return newErrorValueWithPrototype(vmInstance.ErrorPrototype, "AbortError", message)
}

// newTypeErrorValue builds a real TypeError instance - what a network
// failure rejects fetch() with per spec - instead of a bare string (#214).
// Safe to call from any goroutine; see newErrorValueWithPrototype.
func newTypeErrorValue(vmInstance *vm.VM, message string) vm.Value {
	return newErrorValueWithPrototype(vmInstance.TypeErrorPrototype, "TypeError", message)
}

// newTypeError is newTypeErrorValue wrapped as a Go error via
// vmInstance.NewExceptionError, for call sites inside doFetchRequestWithContext
// that return (vm.Value, error) directly rather than settling a promise -
// they too run on fetch()'s background goroutine, so they need the same
// goroutine-safe construction (vmInstance.NewTypeError itself is not safe
// here: it calls the TypeError constructor via vm.Call, which mutates
// shared VM fields with no synchronization against the main interpreter
// loop - see newErrorValueWithPrototype).
func newTypeError(vmInstance *vm.VM, message string) error {
	return vmInstance.NewExceptionError(newTypeErrorValue(vmInstance, message))
}

// bytesToValue wraps raw bytes as a Uint8Array, the representation bytes()
// resolves to (blob() resolves to a real Blob - see NewBlobValue).
func bytesToValue(data []byte) vm.Value {
	arrayBufferValue := vm.NewArrayBuffer(len(data))
	buffer := arrayBufferValue.AsArrayBuffer()
	copy(buffer.GetData(), data)
	return vm.NewTypedArray(vm.TypedArrayUint8, buffer, 0, -1)
}

// blobTypeFromContentType approximates the Fetch spec's Body.blob() "get the
// MIME type" step: the MIME essence - type/subtype, without any
// ";charset=..." parameters - lowercased, or "" if the header is absent.
// Unlike the spec, this doesn't validate the essence as a real MIME type
// (e.g. a garbage header like "not-a-mime-type" passes through unchanged
// instead of yielding ""); good enough for undici's instanceof-based
// Blob-shape checks without a full MIME parser.
func blobTypeFromContentType(contentType string) string {
	essence := strings.TrimSpace(strings.SplitN(contentType, ";", 2)[0])
	return strings.ToLower(essence)
}

// doFetchRequestWithContext performs the HTTP request with context support
// for cancellation. On any early return (parse error, network error, no
// response) it leaves cancel/rt untouched and the promise unsettled - the
// caller must reject the promise with the returned error and only then
// discharge cancel/rt itself: cancel()/rt.EndExternalOp() wake the driver's
// event-loop drain immediately, and if that happened before the promise
// reaction is scheduled, the drain can see nothing pending and return,
// leaving the promise settled but its `await`/`.then()` continuation never
// resumed (previously masked by #213, which made this path effectively dead
// code - see the fetchFn caller for where settlement happens first). On
// success (#205) the response resolves as soon as headers arrive - real
// fetch()/spec semantics, and required so an SSE body that never EOFs
// doesn't block the fetch() promise forever - and this function resolves
// promiseObj itself, synchronously, *before* spawning the goroutine that
// streams the body into a ReadableStream and only then discharges cancel/rt.
// That ordering is required, not cosmetic (#238): the body-drain goroutine
// can finish arbitrarily fast (an empty or already-buffered body EOFs
// immediately) and its rt.EndExternalOp() can otherwise race the promise
// settlement on a completely unsynchronized goroutine - if the external-op
// count reaches zero before the promise is actually resolved, a top-level
// await polling in the gap between those two events sees no pending
// microtasks, no pending external ops, and a still-pending promise, and
// falsely reports deadlock even though the promise was about to settle.
// Resolving here, strictly before the `go func(){...}()` below even starts,
// makes that ordering impossible instead of merely unlikely. The caller
// must NOT resolve/reject promiseObj again in the success case.
func doFetchRequestWithContext(ctx context.Context, cancel context.CancelFunc, rt runtime.AsyncRuntime, vmInstance *vm.VM, promiseObj *vm.PromiseObject, url string, init vm.Value) error {
	// Default options
	method := "GET"
	headers := &FetchHeaders{headers: make(http.Header)}
	var body io.Reader
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
				mergeHeadersFrom(headers, h)
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
							return newTypeError(vmInstance, "failed to serialize body to JSON: "+err.Error())
						}
						body = bytes.NewReader(jsonBytes)
					} else if b.Type() == vm.TypeObject || b.Type() == vm.TypeDictObject {
						// Default to JSON for objects
						jsonBytes, err := b.MarshalJSON()
						if err != nil {
							return newTypeError(vmInstance, "failed to serialize body to JSON: "+err.Error())
						}
						body = bytes.NewReader(jsonBytes)
					} else {
						body = strings.NewReader(b.ToString())
					}
				}
			}

			// Signal (AbortSignal): deliberately *not* re-checked here. This
			// function runs on its own background goroutine (see the call
			// site's doc comment), and "signal" is one of the exposed
			// AbortSignal JS properties fetchFn's own addEventListener
			// registration (in InitRuntime, before this goroutine was even
			// started) already races safely - re-reading signalObj.GetOwn
			// here instead, unsynchronized against a concurrent abort()'s
			// SetOwn, used to be a real (if narrow) data race (#374).
			// Both cases are already handled correctly elsewhere: an
			// already-aborted signal is caught synchronously before this
			// goroutine is ever spawned, and a concurrent/later abort
			// cancels ctx (via that addEventListener registration), which
			// client.Do below turns into an AbortError through ctx.Err().

			// Redirect mode
			if r, exists := initObj.GetOwn("redirect"); exists && r.Type() == vm.TypeString {
				redirectMode = r.ToString()
			}
		}
	}

	// Track if we were redirected
	redirected := false
	originalURL := url

	// Create HTTP client with a timeout on getting a response at all, but not
	// on reading the body afterward - a real (e.g. SSE) stream can run far
	// longer than 30s and must not be killed mid-read (#205).
	client := &http.Client{
		Transport: &http.Transport{
			Proxy:                 FetchProxy,
			ResponseHeaderTimeout: FetchResponseHeaderTimeout,
		},
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) > 0 {
				redirected = true
			}
			switch redirectMode {
			case "error":
				return errors.New("fetch redirect not allowed")
			case "manual":
				return http.ErrUseLastResponse
			default: // "follow"
				if len(via) >= 20 {
					return errors.New("too many redirects")
				}
				return nil
			}
		},
	}

	// Create request with context for cancellation support
	req, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		return err
	}

	// Set headers
	req.Header = headers.headers

	// Perform request - blocks until headers arrive (or ResponseHeaderTimeout
	// fires), not until the body is fully read.
	resp, err := client.Do(req)
	if err != nil {
		// A canceled ctx here means the abort-signal poller genuinely fired
		// cancel() concurrently (this function no longer cancels ctx itself
		// before returning - see the doc comment above, #213) - so this
		// correctly distinguishes a real abort from a plain network error.
		if ctx.Err() == context.Canceled {
			return &AbortError{Message: "The operation was aborted"}
		}
		return err
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

	// Headers are in hand: hand off cancel/rt to the body-drain goroutine
	// below instead of discharging them when this function returns.
	bodyStream, controller := NewHostFedReadableStream(vmInstance)
	bodyState := newFetchBody()

	response := &FetchResponse{
		vm:         vmInstance,
		OK:         resp.StatusCode >= 200 && resp.StatusCode < 300,
		Status:     resp.StatusCode,
		StatusText: resp.Status,
		URL:        respURL,
		Headers:    responseHeaders,
		bodyState:  bodyState,
		bodyStream: bodyStream,
		bodyUsed:   false,
		Redirected: redirected,
		Type:       responseType,
	}

	// Resolve the promise now, strictly before spawning the body-drain
	// goroutine below - see the doc comment above (#238) for why the order
	// matters: this must happen-before that goroutine's rt.EndExternalOp()
	// can possibly run, which is only guaranteed if it has not even been
	// spawned yet.
	vmInstance.ResolvePromise(promiseObj, createResponseObject(vmInstance, response))

	// Stream the body in off the VM goroutine: enqueue each chunk on the
	// ReadableStream as it arrives (for consumers reading response.body
	// directly) and accumulate it in bodyState (for text()/json()/etc,
	// which still want the whole thing at once). ctx/cancel/resp.Body and
	// the external op all now live for exactly this goroutine's lifetime.
	go func() {
		defer cancel()
		defer rt.EndExternalOp()
		defer resp.Body.Close()

		buf := make([]byte, 32*1024)
		for {
			n, readErr := resp.Body.Read(buf)
			if n > 0 {
				chunk := append([]byte(nil), buf[:n]...)
				controller.EnqueueBytes(chunk)
				bodyState.appendChunk(chunk)
			}
			if readErr != nil {
				if readErr == io.EOF {
					controller.Close()
					bodyState.finish(nil)
				} else {
					controller.Error(vm.NewString(readErr.Error()))
					bodyState.finish(readErr)
				}
				return
			}
		}
	}()

	return nil
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
	mergeHeadersFrom(headers, v)
	return headers
}

// mergeHeadersFrom reads header entries out of v - which may be a real
// Headers instance, or a plain object/dict literal such as
// { "Content-Type": "application/json" } - into dst.
//
// A Headers instance's own data lives behind SetInternalSlots (see
// createHeadersObject): every one of its get/set/append/etc. accessors is
// installed non-enumerable, so OwnKeys() on it is always empty. Reading
// headers via OwnKeys() alone therefore silently drops everything set
// through a real Headers object's .set()/.append(), or passed as
// new Headers(anotherHeadersInstance) - #237. Check for that internal state
// first and only fall back to OwnKeys() for genuine object/dict literals.
func mergeHeadersFrom(dst *FetchHeaders, v vm.Value) {
	if v.Type() == vm.TypeObject {
		obj := v.AsPlainObject()
		if h, ok := obj.InternalSlots().(*FetchHeaders); ok && h != nil {
			for name, values := range h.headers {
				for _, value := range values {
					dst.headers.Add(name, value)
				}
			}
			return
		}
		for _, key := range obj.OwnKeys() {
			if val, exists := obj.GetOwn(key); exists {
				dst.headers.Set(key, val.ToString())
			}
		}
	} else if v.Type() == vm.TypeDictObject {
		dictObj := v.AsDictObject()
		for _, key := range dictObj.OwnKeys() {
			if val, exists := dictObj.GetOwn(key); exists {
				dst.headers.Set(key, val.ToString())
			}
		}
	}
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

	// Stash the underlying *FetchRequest behind an internal slot (same
	// pattern createHeadersObject uses for *FetchHeaders) so fetch() can
	// recover the full request - including its body bytes and Signal,
	// neither of which round-trip through the JS-visible properties below
	// ("body" is always exposed as null; "signal" here is whatever value
	// was passed in, but reading it back this way is more direct and
	// matches how mergeHeadersFrom already prefers internal state) -
	// instead of stringifying the Request object itself into a URL.
	obj.SetInternalSlots(req)

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
			return vm.Undefined, vmInstance.NewTypeError("Request body is already used")
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

		mimeType := blobTypeFromContentType(req.Headers.headers.Get("Content-Type"))
		if req.body == nil {
			return vmInstance.NewResolvedPromise(NewBlobValue(vmInstance, nil, mimeType)), nil
		}

		return vmInstance.NewResolvedPromise(NewBlobValue(vmInstance, req.body, mimeType)), nil
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

	// formData() -> Promise<FormData> (#397)
	obj.SetOwnNonEnumerable("formData", vm.NewNativeFunction(0, false, "formData", func(args []vm.Value) (vm.Value, error) {
		if req.bodyUsed {
			return vmInstance.NewRejectedPromise(vm.NewString("body already used")), nil
		}
		req.bodyUsed = true
		obj.SetOwn("bodyUsed", vm.True)

		fdValue, err := parseMultipartFormData(vmInstance, req.Headers.headers.Get("Content-Type"), req.body)
		if err != nil {
			return vmInstance.NewRejectedPromise(vm.NewString(err.Error())), nil
		}
		return vmInstance.NewResolvedPromise(fdValue), nil
	}))

	return vm.NewValueFromPlainObject(obj)
}
