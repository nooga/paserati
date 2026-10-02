package vm

import "sync"

// Unhandled promise rejection tracking (#120).
//
// A promise that is rejected while it has no reject reaction is recorded. A
// reaction attached later (a .then/.catch/await, even from a microtask of the
// same turn) marks it handled. When the microtask queue drains, the promises
// still unhandled are handed to the host's hook, matching the point at which
// Node raises 'unhandledRejection'.
//
// Tracking is off until a host installs a hook with
// SetUnhandledRejectionHandler: an embedder or test runner that doesn't ask
// for the report sees no behaviour change.

// UnhandledRejectionHandler is called once per promise that was rejected and
// still has no handler when the microtask queue drains.
type UnhandledRejectionHandler func(reason Value, promise Value)

type rejectionTracker struct {
	mu      sync.Mutex
	hook    UnhandledRejectionHandler
	pending []*PromiseObject
}

// SetUnhandledRejectionHandler installs (or, with nil, removes) the hook that
// receives unhandled rejections.
func (vm *VM) SetUnhandledRejectionHandler(h UnhandledRejectionHandler) {
	vm.rejections.mu.Lock()
	defer vm.rejections.mu.Unlock()
	vm.rejections.hook = h
	if h == nil {
		vm.rejections.pending = nil
	}
}

// trackRejection records a promise that has just been rejected. Safe to call
// from any goroutine.
func (vm *VM) trackRejection(p *PromiseObject) {
	vm.rejections.mu.Lock()
	defer vm.rejections.mu.Unlock()
	if vm.rejections.hook == nil {
		return
	}
	p.mu.Lock()
	handled := p.handled
	p.mu.Unlock()
	if !handled {
		vm.rejections.pending = append(vm.rejections.pending, p)
	}
}

// ProcessUnhandledRejections reports every tracked promise that is still
// unhandled and reports whether it called the hook at all. The host event
// loop calls it each time the microtask queue drains.
func (vm *VM) ProcessUnhandledRejections() bool {
	vm.rejections.mu.Lock()
	hook := vm.rejections.hook
	pending := vm.rejections.pending
	vm.rejections.pending = nil
	vm.rejections.mu.Unlock()
	if hook == nil || len(pending) == 0 {
		return false
	}
	reported := false
	for _, p := range pending {
		p.mu.Lock()
		handled, reason := p.handled, p.Result
		p.mu.Unlock()
		if handled {
			continue
		}
		reported = true
		hook(reason, Value{typ: TypePromise, obj: promiseToUnsafe(p)})
	}
	return reported
}

// FormatUnhandledRejection formats a rejection reason the way an uncaught
// exception is displayed, with the established "Uncaught (in promise)" prefix.
func (vm *VM) FormatUnhandledRejection(reason Value) string {
	msg := "Uncaught (in promise): " + vm.formatExceptionDisplay(reason)
	if reason.Type() == TypeObject {
		if stackVal, ok := reason.AsPlainObject().GetOwn("stack"); ok {
			if stack := stackVal.ToString(); stack != "" {
				msg += "\n" + stack
			}
		}
	}
	return msg
}
