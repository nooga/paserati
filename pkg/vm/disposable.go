package vm

// DisposableResource is one entry of a DisposableStack/AsyncDisposableStack's
// dispose capability (the spec's [[DisposableResourceStack]]): a value and
// the method to invoke on it during disposal. Disposal calls Method with
// `this` set to Value and no arguments, per the spec's Dispose(V, hint,
// method) abstract operation.
type DisposableResource struct {
	Value  Value
	Method Value // always callable
}

// DisposableStackState is the [[DisposableState]]/[[DisposeCapability]]
// internal slots behind a DisposableStack or AsyncDisposableStack instance.
// Resources are recorded in push order and disposed in reverse.
type DisposableStackState struct {
	Disposed  bool
	Async     bool // true for AsyncDisposableStack; affects GetDisposeMethod's hint
	Resources []DisposableResource
}
