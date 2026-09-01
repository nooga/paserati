package driver

import (
	"errors"
	"testing"

	"github.com/nooga/paserati/pkg/vm"
)

func TestHostAsyncFunctionReturnsPromise(t *testing.T) {
	p := NewPaserati()
	p.SetSkipTypeCheck(true)
	p.DeclareModule("io", func(m *ModuleBuilder) {
		m.AsyncFunction("read", func() string {
			return "hello"
		})
	})

	js := `
		import { read } from "io";
		const p = read();
		const thenable = typeof p.then === "function";
		const val = await read();
		thenable && val === "hello"
	`
	result, errs := p.RunCode(js, RunOptions{})
	if len(errs) > 0 {
		t.Fatalf("RunCode failed: %v", errs[0])
	}
	if !result.IsTruthy() {
		t.Errorf("expected thenable Promise resolving to \"hello\", got %v", result.ToString())
	}
}

func TestHostAsyncFunctionRejectsOnError(t *testing.T) {
	p := NewPaserati()
	p.SetSkipTypeCheck(true)
	p.DeclareModule("io", func(m *ModuleBuilder) {
		m.AsyncFunction("fail", func() (string, error) {
			return "", errors.New("boom")
		})
	})

	js := `
		import { fail } from "io";
		await (async () => {
			try {
				await fail();
				return "did-not-throw";
			} catch (e) {
				return String(e).includes("boom") ? "caught" : String(e);
			}
		})()
	`
	result, errs := p.RunCode(js, RunOptions{})
	if len(errs) > 0 {
		t.Fatalf("RunCode failed: %v", errs[0])
	}
	if result.ToString() != "caught" {
		t.Errorf("expected catch of rejected promise, got %v", result.ToString())
	}
}

// richExceptionError is a host-supplied Go error that carries a real JS value,
// exactly the shape a host uses to throw a Node-style SystemError with .code /
// .errno / .syscall attached (noderati's fs/promises is the motivating case).
type richExceptionError struct{ v vm.Value }

func (e *richExceptionError) Error() string               { return "boom" }
func (e *richExceptionError) GetExceptionValue() vm.Value { return e.v }

// TestHostAsyncFunctionRejectsWithRealExceptionValue covers #147: an
// AsyncFunction's Go error that implements vm.ExceptionError must reject the
// promise with the *real* value it carries, the way the synchronous
// ModuleBuilder.Function path already throws it. It used to be flattened to
// vm.NewString(err.Error()), so a rejection handler received a bare JS string
// and every property the host had put on its Error object was gone.
func TestHostAsyncFunctionRejectsWithRealExceptionValue(t *testing.T) {
	p := NewPaserati()
	p.SetSkipTypeCheck(true)

	newRich := func() error {
		vmInst := p.GetVM()
		obj := vm.NewObject(vmInst.ErrorPrototype).AsPlainObject()
		obj.SetOwn("name", vm.NewString("Error"))
		obj.SetOwn("message", vm.NewString("no such file or directory"))
		obj.SetOwn("code", vm.NewString("ENOENT"))
		return &richExceptionError{v: vm.NewValueFromPlainObject(obj)}
	}

	p.DeclareModule("io", func(m *ModuleBuilder) {
		m.Function("statSync", func() (interface{}, error) { return nil, newRich() })
		m.AsyncFunction("stat", func() (interface{}, error) { return nil, newRich() })
	})

	js := `
		import { stat, statSync } from "io";
		let sync = "no-throw";
		try { statSync(); } catch (e) { sync = typeof e + ":" + e.code; }
		let async = "no-throw";
		try { await stat(); } catch (e) { async = typeof e + ":" + e.code; }
		sync + "|" + async
	`
	result, errs := p.RunCode(js, RunOptions{})
	if len(errs) > 0 {
		t.Fatalf("RunCode failed: %v", errs[0])
	}
	// Both paths must agree, and both must hand back the real Error object.
	if got := result.ToString(); got != "object:ENOENT|object:ENOENT" {
		t.Errorf("sync|async rejection value mismatch: got %q, want %q", got, "object:ENOENT|object:ENOENT")
	}
}
