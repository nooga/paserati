package driver

import (
	"errors"
	"testing"
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
