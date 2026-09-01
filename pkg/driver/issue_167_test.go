package driver

import (
	"errors"
	"testing"
)

type issue167Thing struct{ Value string }

func newIssue167Thing(input string) (*issue167Thing, error) {
	if input == "" {
		return nil, errors.New("input required")
	}
	return &issue167Thing{Value: input}, nil
}

// TestClassConstructorErrorReturnThrows covers paserati#167:
// ModuleBuilder.createClassConstructor only ever looked at results[0] from
// a Go constructor call, silently discarding a second (error) return value.
// A constructor with the idiomatic Go shape func(...) (*T, error) that
// returned a non-nil error made `new X(...)` evaluate to undefined
// instead of throwing - the same (value, error) handling goFunctionToVM
// already had for plain functions was missing here.
func TestClassConstructorErrorReturnThrows(t *testing.T) {
	p := NewPaserati()
	p.DeclareModule("issue167mod", func(m *ModuleBuilder) {
		m.Class("Thing", &issue167Thing{}, newIssue167Thing)
	})

	res, errs := p.RunString(`
		import { Thing } from "issue167mod";
		let threw = false;
		let message = "";
		try {
			new Thing("");
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
		t.Fatalf("expected constructor error to throw: got %s, want %s", got, want)
	}
}

// TestClassConstructorErrorReturnSucceedsWithoutError guards against a
// regression: the (value, error) success case - error is nil - must still
// return a working instance, not treat every two-value constructor as an
// error path.
func TestClassConstructorErrorReturnSucceedsWithoutError(t *testing.T) {
	p := NewPaserati()
	p.DeclareModule("issue167mod2", func(m *ModuleBuilder) {
		m.Class("Thing", &issue167Thing{}, newIssue167Thing)
	})

	res, errs := p.RunString(`
		import { Thing } from "issue167mod2";
		const t = new Thing("hello");
		t.Value;
	`)
	if len(errs) > 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	if res.ToString() != "hello" {
		t.Fatalf("expected instance field 'hello', got %s", res.ToString())
	}
}
