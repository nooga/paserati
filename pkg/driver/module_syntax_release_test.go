package driver

import (
	"strings"
	"testing"

	"github.com/nooga/paserati/pkg/modules"
)

// A compiled module keeps no AST, yet everything that reads like source still
// works: Function.prototype.toString, stack traces with file and position, and
// linking later importers (star re-exports, top-level await, link errors).
func TestModuleSyntaxReleasedAfterCompile(t *testing.T) {
	p := newModuleSession(t, map[string]string{
		"lib":    "export function add(a: number, b: number) {\n  return a + b; // sum\n}\nexport class Box { m() { return 1; } }\nexport function boom(): never {\n  throw new Error(\"x\");\n}\nexport const arrow = (x: number) => x * 2;",
		"barrel": `export * from "lib"; export * as ns from "lib"; export { add as plus } from "lib";`,
		"tla":    `export const v = await Promise.resolve(3);`,
	})
	c, _ := p.NewContext()
	got := runIn(t, c, 0, `import { add, Box, boom, arrow } from "lib";
import { plus, ns } from "barrel";
import { v } from "tla";
let stack = "";
try { boom(); } catch (e) { stack = (e as Error).stack || ""; }
[add.toString(), Box.toString(), arrow.toString(), plus(1, 2), Object.keys(ns).length, v, stack.indexOf("lib") >= 0 && /:6:/.test(stack)].join("|")`)
	for _, want := range []string{"function add(a: number, b: number) {\n  return a + b; // sum\n}", "class Box { m() { return 1; } }", "(x: number) => x * 2", "|3|", "|4|", "|3|true"} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %q in %q", want, got)
		}
	}
	// The graph's syntax is gone; a later Context still links and runs it.
	for _, name := range []string{"lib", "barrel", "tla"} {
		rec, err := p.moduleLoader.LoadModule(name, "/entry0.ts")
		if err != nil {
			t.Fatal(err)
		}
		if r := rec.(*modules.ModuleRecord); r.AST != nil || r.LinkSummary == nil {
			t.Fatalf("%s: AST retained=%v, summary=%v", name, r.AST != nil, r.LinkSummary != nil)
		}
	}
	c2, _ := p.NewContext()
	if got := runIn(t, c2, 1, `import { plus } from "barrel"; import * as b from "barrel"; plus(2, 2) + "," + Object.keys(b).sort().join()`); !strings.HasPrefix(got, "4,") {
		t.Fatalf("second context: %q", got)
	}
	// A bad import of a released module is still a positioned link error.
	_, errs := c2.RunCode("import { nope } from \"lib\";\nnope;", RunOptions{ModuleName: "/bad.ts", Filename: "/bad.ts"})
	if len(errs) == 0 {
		t.Fatal("importing a missing export should fail")
	}
}
