package driver

import (
	"errors"
	"testing"

	"github.com/nooga/paserati/pkg/vm"
)

type issue221Thing struct{ Value string }

func newIssue221Thing(input string) (*issue221Thing, error) {
	return &issue221Thing{Value: input}, nil
}

// DoThing returns an error when its argument is empty - the shape
// createBoundMethod is supposed to special-case, just like
// createClassConstructor already does for constructors (paserati#167).
func (t *issue221Thing) DoThing(input string) (vm.Value, error) {
	if input == "" {
		return vm.Undefined, errors.New("input required")
	}
	return vm.NewString(t.Value + input), nil
}

// TestBoundMethodErrorReturnThrows covers paserati#221:
// ModuleBuilder.createBoundMethod only ever looked at results[0] from a
// bound Go method call, silently discarding a second (error) return value -
// unlike goFunctionToVM (module-level Function) and createClassConstructor
// (Class constructor), both of which already special-case the idiomatic Go
// (T, error) return shape. A bound instance method returning a non-nil
// error made the call evaluate to a value instead of throwing.
func TestBoundMethodErrorReturnThrows(t *testing.T) {
	p := NewPaserati()
	p.DeclareModule("issue221mod", func(m *ModuleBuilder) {
		m.Class("Thing", &issue221Thing{}, newIssue221Thing)
	})

	res, errs := p.RunString(`
		import { Thing } from "issue221mod";
		let threw = false;
		let message = "";
		const t = new Thing("hello");
		try {
			t.doThing("");
		} catch (e) {
			threw = true;
			message = e.message;
		}
		JSON.stringify({ threw, message });
	`)
	if len(errs) > 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	got := res.ToString()
	want := `{"threw":true,"message":"input required"}`
	if got != want {
		t.Fatalf("expected bound method error to throw: got %s, want %s", got, want)
	}
}

// TestBoundMethodErrorReturnSucceedsWithoutError guards against a
// regression: the (value, error) success case - error is nil - must still
// return the method's value, not treat every two-value method as an error
// path.
func TestBoundMethodErrorReturnSucceedsWithoutError(t *testing.T) {
	p := NewPaserati()
	p.DeclareModule("issue221mod2", func(m *ModuleBuilder) {
		m.Class("Thing", &issue221Thing{}, newIssue221Thing)
	})

	res, errs := p.RunString(`
		import { Thing } from "issue221mod2";
		const t = new Thing("hello ");
		t.doThing("world");
	`)
	if len(errs) > 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	if res.ToString() != "hello world" {
		t.Fatalf("expected 'hello world', got %s", res.ToString())
	}
}
