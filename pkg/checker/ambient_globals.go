package checker

import "github.com/nooga/paserati/pkg/types"

// tsc's default lib (lib.d.ts) declares the DOM globals and a number of
// ES library types that Paserati has no runtime for. Programs are type checked
// against that lib, so naming them is not an error; they are ambient and
// any-typed here (a use that reaches a missing runtime global fails when it
// runs, not when it is checked).

// ambientValueGlobals are `declare var`/`declare function` names from lib.dom.
var ambientValueGlobals = []string{
	"window", "self", "document", "navigator", "location", "history", "screen",
	"localStorage", "sessionStorage", "performance", "frames", "parent", "top",
	"setTimeout", "clearTimeout", "setInterval", "clearInterval",
	"requestAnimationFrame", "cancelAnimationFrame", "requestIdleCallback",
	"alert", "confirm", "prompt", "fetch", "innerWidth", "innerHeight",
	"XMLHttpRequest", "WebSocket", "Worker", "Blob", "File", "FileReader", "FormData",
	"Headers", "Request", "Response", "URLSearchParams", "MutationObserver",
	"Event", "CustomEvent", "MouseEvent", "KeyboardEvent", "CloseEvent", "MessageEvent",
	"Node", "Element", "Document", "Window", "Text", "Image", "Audio",
	"HTMLElement", "HTMLCanvasElement", "HTMLDivElement", "HTMLSpanElement",
	"HTMLAnchorElement", "HTMLImageElement", "HTMLInputElement", "HTMLButtonElement",
	"HTMLFormElement", "HTMLTableElement", "HTMLSelectElement", "HTMLTextAreaElement",
	"SVGElement", "SVGSVGElement", "SVGPathElement",
	"CSSStyleDeclaration", "DOMTokenList", "NodeList", "HTMLCollection",
}

// ambientTypeGlobals are interface/alias names from lib.dom and lib.es5 that
// have no Paserati type; they resolve to any.
var ambientTypeGlobals = []string{
	"PropertyDescriptorMap", "RegExpExecArray", "RegExpMatchArray",
	"ClassDecorator", "MethodDecorator", "PropertyDecorator", "ParameterDecorator",
	"ArrayBufferLike", "ArrayBufferView", "CallableFunction", "NewableFunction",
	"PromiseConstructorLike", "SymbolConstructor", "CloseEvent", "MessageEvent",
	"EventTarget", "EventListener", "AddEventListenerOptions", "RequestInit", "ResponseInit",
	"HTMLElement", "HTMLCanvasElement", "HTMLDivElement", "HTMLSpanElement",
	"HTMLAnchorElement", "HTMLImageElement", "HTMLInputElement", "HTMLButtonElement",
	"HTMLFormElement", "HTMLTableElement", "HTMLSelectElement", "HTMLTextAreaElement",
	"SVGElement", "SVGSVGElement", "SVGPathElement", "Event", "MouseEvent",
	"KeyboardEvent", "CustomEvent", "Node", "Element", "Document", "Window",
	"CSSStyleDeclaration", "DOMTokenList", "NodeList", "HTMLCollection",
	"XMLHttpRequest", "WebSocket", "Worker", "File", "Blob", "URL",
}

// declareAmbientLibGlobals binds the lib names above that the builtin
// initializers did not already provide.
func declareAmbientLibGlobals(env *Environment) {
	for _, name := range ambientValueGlobals {
		if _, _, found := env.Resolve(name); !found {
			env.Define(name, types.Any, false)
		}
	}
	for _, name := range ambientTypeGlobals {
		if _, found := env.ResolveType(name); !found {
			env.DefineTypeAlias(name, types.Any)
		}
	}
	// lib.es5: type PropertyKey = string | number | symbol
	if _, found := env.ResolveType("PropertyKey"); !found {
		env.DefineTypeAlias("PropertyKey", types.NewUnionType(types.String, types.Number, types.Symbol))
	}
	// lib.es5: interface PropertyDescriptor
	if _, found := env.ResolveType("PropertyDescriptor"); !found {
		pd := types.NewObjectType()
		for _, name := range []string{"configurable", "enumerable", "writable"} {
			pd.Properties[name] = types.Boolean
			pd.OptionalProperties[name] = true
		}
		pd.Properties["value"] = types.Any
		pd.OptionalProperties["value"] = true
		pd.Properties["get"] = types.Any
		pd.OptionalProperties["get"] = true
		pd.Properties["set"] = types.Any
		pd.OptionalProperties["set"] = true
		env.DefineTypeAlias("PropertyDescriptor", pd)
	}
}

// isLibGlobalValue reports whether name is a value the builtin initializers
// declared in the root global scope (as opposed to one the program declares).
func (c *Checker) isLibGlobalValue(name string) bool {
	for e := c.env; e != nil; e = e.outer {
		if _, ok := e.symbols[name]; ok {
			// Script top-level declarations also live in the root scope, so
			// being there is not enough: it must predate this program.
			return e.outer == nil && c.preexistingGlobals[name]
		}
	}
	return false
}

// libGenericTypeNames are generic interfaces/aliases of tsc's default lib that
// Paserati may not model; naming one is never an unresolved name.
var libGenericTypeNames = map[string]bool{
	"Array": true, "ReadonlyArray": true, "Promise": true, "PromiseLike": true, "Map": true, "Set": true,
	"WeakMap": true, "WeakSet": true, "WeakRef": true, "ReadonlyMap": true, "ReadonlySet": true,
	"Iterable": true, "Iterator": true, "IterableIterator": true, "IteratorResult": true,
	"IteratorYieldResult": true, "IteratorReturnResult": true, "Generator": true,
	"AsyncGenerator": true, "AsyncIterable": true, "AsyncIterator": true, "AsyncIterableIterator": true,
	"IterableIteratorObject": true, "IteratorObject": true, "AsyncIteratorObject": true, "ArrayIterator": true,
	"MapIterator": true, "SetIterator": true, "StringIterator": true, "RegExpStringIterator": true,
	"ReadonlyArrayConstructor": true, "PromiseConstructorLike": true, "Disposable": true, "AsyncDisposable": true, "ArrayLike": true, "Record": true, "Partial": true, "Required": true,
	"Readonly": true, "Pick": true, "Omit": true, "Exclude": true, "Extract": true, "NonNullable": true,
	"Parameters": true, "ConstructorParameters": true, "ReturnType": true, "InstanceType": true,
	"ThisParameterType": true, "OmitThisParameter": true, "ThisType": true, "Awaited": true,
	"Uppercase": true, "Lowercase": true, "Capitalize": true, "Uncapitalize": true, "NoInfer": true,
	"PromiseSettledResult": true, "PromiseFulfilledResult": true, "PromiseRejectedResult": true,
	"ProxyHandler": true, "TypedPropertyDescriptor": true, "PropertyDescriptor": true,
	"NodeListOf": true, "HTMLCollectionOf": true, "FinalizationRegistry": true, "Atomics": true,
	"SharedArrayBuffer": true, "ArrayBufferConstructor": true, "DataView": true, "EventListenerOrEventListenerObject": true,
	"TemplateStringsArray": true, "RegExpMatchArray": true, "RegExpExecArray": true,
	"Int8Array": true, "Uint8Array": true, "Uint8ClampedArray": true, "Int16Array": true, "Uint16Array": true,
	"Int32Array": true, "Uint32Array": true, "Float32Array": true, "Float64Array": true,
	"BigInt64Array": true, "BigUint64Array": true,
}

// mayBeDeclaredGeneric reports whether an unresolved generic type name could
// still be defined: declared somewhere in the program, imported, an in-scope
// type parameter, or a known lib generic.
func (c *Checker) mayBeDeclaredGeneric(name string) bool {
	if c.declaredTypeNames[name] || libGenericTypeNames[name] {
		return true
	}
	if _, ok := c.env.ResolveTypeParameter(name); ok {
		return true
	}
	if c.IsModuleMode() && c.moduleEnv != nil {
		if t := c.moduleEnv.ResolveImportedType(name); t != nil {
			return true
		}
	}
	// Imports are bound lazily; any binding or alias of that name counts.
	if _, _, found := c.env.Resolve(name); found {
		return true
	}
	return false
}
