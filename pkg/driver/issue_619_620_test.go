package driver

import (
	"strings"
	"testing"
)

func declareSumModule(p *Paserati) {
	p.DeclareModule("host", func(m *ModuleBuilder) {
		m.Function("sum", func(a, b float64) float64 { return a + b })
	})
}

// An import in Script mode is a SyntaxError returned in errs, not a panic
// out of RunCode (#620).
func TestScriptModeImportIsSyntaxError(t *testing.T) {
	for _, typecheck := range []bool{true, false} {
		for _, src := range []string{
			`import { sum } from "host"; sum(1, 2)`,
			`export const x = 1;`,
		} {
			p := NewPaserati()
			p.SetSkipTypeCheck(!typecheck)
			declareSumModule(p)
			_, errs := p.RunCode(src, RunOptions{Script: true})
			if len(errs) == 0 {
				t.Fatalf("typecheck=%v %q: expected a SyntaxError, got none", typecheck, src)
			}
			if msg := errs[0].Error(); !strings.Contains(msg, "SyntaxError") || !strings.Contains(msg, "outside a module") {
				t.Fatalf("typecheck=%v %q: unexpected error %q", typecheck, src, msg)
			}
		}
	}
}

// Importing a name a native module does not export is a link-time
// SyntaxError, not undefined (#619).
func TestNativeModuleMissingExportIsLinkError(t *testing.T) {
	for _, typecheck := range []bool{true, false} {
		p := NewPaserati()
		p.SetSkipTypeCheck(!typecheck)
		declareSumModule(p)
		_, errs := p.RunCode(`import { nope } from "host"; String(typeof nope)`, RunOptions{})
		if len(errs) == 0 {
			t.Fatalf("typecheck=%v: expected a link error, got none", typecheck)
		}
		if typecheck {
			continue // the checker reports it first, in its own words
		}
		if msg := errs[0].Error(); !strings.Contains(msg, "does not provide an export named 'nope'") {
			t.Fatalf("unexpected error %q", msg)
		}
	}

	p := NewPaserati()
	p.SetSkipTypeCheck(true)
	declareSumModule(p)
	v, errs := p.RunCode(`import { sum } from "host"; import * as ns from "host"; sum(1, 2) + ns.sum(3, 4)`, RunOptions{})
	if len(errs) > 0 {
		t.Fatalf("valid import failed: %v", errs[0])
	}
	if v.ToString() != "10" {
		t.Fatalf("got %s, want 10", v.ToString())
	}
}
