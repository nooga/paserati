// Package temporal is the engine-independent core of the Temporal built-ins:
// ISO 8601 calendar arithmetic, durations, rounding, string parsing and
// formatting, and IANA time zone support. It knows nothing about the VM; the
// JS-visible objects in pkg/builtins are thin bindings over it, so the
// spec's abstract operations can be unit tested directly.
//
// Names follow the Temporal proposal's abstract operations
// (https://tc39.es/proposal-temporal/) wherever there is one. Only the ISO
// 8601 calendar is supported.
//
// Errors are plain Go errors. The bindings turn a *RangeError into a JS
// RangeError and a *TypeError into a JS TypeError, because the spec chooses
// between the two per operation.
package temporal
