package driver

import (
	"fmt"
	"testing"

	"github.com/nooga/paserati/pkg/modules"
)

// A module graph loaded through one Context is instantiated afresh in each
// other Context of the session: module records and namespaces are per realm
// (as in the spec), while parsing, checking and compiling stay once per
// session.

func newModuleSession(t *testing.T, mods map[string]string) *Paserati {
	t.Helper()
	p := NewPaserati()
	t.Cleanup(p.Cleanup)
	mem := modules.NewMemoryResolver("mem")
	for name, src := range mods {
		mem.AddModule(name, src)
	}
	p.AddResolver(mem)
	return p
}

func runIn(t *testing.T, c *Context, i int, src string) string {
	t.Helper()
	name := fmt.Sprintf("/entry%d.ts", i)
	v, errs := c.RunCode(src, RunOptions{ModuleName: name, Filename: name})
	if len(errs) > 0 {
		t.Fatalf("context run %d: %v", i, errs)
	}
	return v.ToString()
}

func TestContextModulesPerRealm(t *testing.T) {
	p := newModuleSession(t, map[string]string{
		"lib": `let calls = 0; export function bump(): number { calls++; return calls; }`,
	})
	for i := 0; i < 3; i++ {
		c, err := p.NewContext()
		if err != nil {
			t.Fatal(err)
		}
		if got := runIn(t, c, i, `import { bump } from "lib"; bump(); bump();`); got != "2" {
			t.Fatalf("context %d: got %s, want 2 (fresh module instance)", i, got)
		}
	}
}

func TestContextModulesStayIsolated(t *testing.T) {
	p := newModuleSession(t, map[string]string{
		"state": `export const box = { n: 0 }; export class Counter { static made = 0; constructor() { Counter.made++; } }`,
		"dep":   `import { box } from "state"; export function inc(): number { return ++box.n; }`,
	})
	a, _ := p.NewContext()
	b, _ := p.NewContext()
	if got := runIn(t, a, 0, `import { inc } from "dep"; inc(); inc(); (globalThis as any).marker = 1; inc()`); got != "3" {
		t.Fatalf("a: got %s, want 3", got)
	}
	// b sees neither a's module state, nor its globals, nor its prototypes.
	if got := runIn(t, b, 1, `import { inc } from "dep"; String(inc()) + typeof (globalThis as any).marker`); got != "1undefined" {
		t.Fatalf("b: got %s, want 1undefined", got)
	}
	// a keeps its own state afterwards.
	if got := runIn(t, a, 2, `import { inc } from "dep"; inc()`); got != "4" {
		t.Fatalf("a again: got %s, want 4", got)
	}
	// Classes are per realm too.
	if got := runIn(t, a, 3, `import { Counter } from "state"; new Counter(); new Counter(); Counter.made`); got != "2" {
		t.Fatalf("a class: got %s, want 2", got)
	}
	if got := runIn(t, b, 4, `import { Counter } from "state"; new Counter(); Counter.made`); got != "1" {
		t.Fatalf("b class: got %s, want 1", got)
	}
	if got := runIn(t, b, 5, `import * as s from "state"; import { box } from "state"; String(s.box === box) + (Object.getPrototypeOf(box) === Object.prototype)`); got != "truetrue" {
		t.Fatalf("b namespace: got %s", got)
	}
}

func TestContextModulesAcrossShapes(t *testing.T) {
	p := newModuleSession(t, map[string]string{
		"base":      `export const green = 1; export function colorName(c: number): string { return ["Red", "Green"][c]; } export const tag = "base"; export default function id<T>(x: T): T { return x; }`,
		"mid":       `export { green, colorName, tag as baseTag } from "base"; export * from "base"; import id from "base"; export const mid = id(5);`,
		"cycleA":    `import { b } from "cycleB"; export const a = 1; export function sum() { return a + b; }`,
		"cycleB":    `import { a } from "cycleA"; export const b = 2; export function peek() { return a; }`,
		"data.json": `{"k": 7}`,
	})
	src := `
import id from "base";
import { green, colorName, baseTag, mid } from "mid";
import * as ns from "mid";
import { sum } from "cycleA";
import data from "data.json";
const dyn = await import("base");
[id(1), green, colorName(1), baseTag, mid, Object.keys(ns).length > 0, sum(), data.k, dyn.tag].join(",")`
	want := "1,1,Green,base,5,true,3,7,base"
	for i := 0; i < 3; i++ {
		c, _ := p.NewContext()
		if got := runIn(t, c, i, src); got != want {
			t.Fatalf("context %d: got %s, want %s", i, got, want)
		}
	}
}

func TestContextModulesNative(t *testing.T) {
	p := NewPaserati()
	defer p.Cleanup()
	p.DeclareModule("host", func(m *ModuleBuilder) {
		m.Function("twice", func(n int) int { return n * 2 })
		m.Default(nil)
	})
	for i := 0; i < 2; i++ {
		c, _ := p.NewContext()
		if got := runIn(t, c, i, `import { twice } from "host"; twice(21)`); got != "42" {
			t.Fatalf("context %d: got %s, want 42", i, got)
		}
	}
}

func TestContextModulesDefaultRealmUnchanged(t *testing.T) {
	p := newModuleSession(t, map[string]string{
		"lib": `let calls = 0; export function bump(): number { calls++; return calls; }`,
	})
	// The session's own realm keeps its module map across RunCode calls.
	for want := 1; want <= 2; want++ {
		v, errs := p.RunCode(`import { bump } from "lib"; bump();`, RunOptions{ModuleName: "/main.ts", Filename: "/main.ts"})
		if len(errs) > 0 {
			t.Fatal(errs)
		}
		if got := v.ToString(); got != fmt.Sprint(want) {
			t.Fatalf("default realm run %d: got %s, want %d", want, got, want)
		}
	}
	// A Context created afterwards starts from scratch.
	c, _ := p.NewContext()
	if got := runIn(t, c, 0, `import { bump } from "lib"; bump();`); got != "1" {
		t.Fatalf("context: got %s, want 1", got)
	}
}

// A module exercising the constant kinds a chunk can hold (functions, classes,
// regexes, tagged templates, bigints, generators, async code, getters,
// namespaces) must be instantiable, so each Context gets a private copy.
func TestContextModulesKitchenSink(t *testing.T) {
	p := newModuleSession(t, map[string]string{
		"sink": `
const re = /a(b+)c/g;
function tag(s: TemplateStringsArray, ...v: number[]) { return s.raw.join("|") + v.join(","); }
class Base { x = 1; get double() { return this.x * 2; } static make() { return new Base(); } #p = 5; priv() { return this.#p; } }
class Derived extends Base { constructor() { super(); this.x = 4; } }
function* gen() { yield 1; yield 2; }
async function later() { return 7; }
let n = 0;
export function run(): string {
  n++;
  const m = re.exec("xabbc");
  const arr = [...gen()];
  return [m![1], tag` + "`a${1}b${2}`" + `, 10n ** 3n, new Derived().double, Base.make().priv(), arr.join(""), 4, n].join(";");
}
export const p = later();
export const obj = { get g() { return 9; }, [Symbol.iterator]: null, m() { return 1; } };
`,
	})
	want := "bb;a|b|1,2;1000;8;5;12;4;1"
	for i := 0; i < 3; i++ {
		c, _ := p.NewContext()
		got := runIn(t, c, i, `import { run, p, obj } from "sink"; const r = run(); r + ":" + obj.g + (await p)`)
		if got != want+":97" {
			t.Fatalf("context %d: got %q, want %q", i, got, want+":97")
		}
	}
	if n := p.vmInstance.SharedModuleChunkFallbacks(); n != 0 {
		t.Fatalf("%d module chunks could not be instantiated per realm", n)
	}
}

// A native module's exports are built per Context, so a script mutating an
// exported object cannot be seen from another Context; Shared() opts a module
// out (one set of values for the whole session).
func TestContextModulesNativeIsolationAndShared(t *testing.T) {
	for _, shared := range []bool{false, true} {
		builds := 0
		p := NewPaserati()
		nm := p.DeclareModule("host", func(m *ModuleBuilder) {
			builds++
			m.Const("config", map[string]interface{}{"level": 1})
			m.Function("twice", func(n int) int { return n * 2 })
			m.Default(nil)
		})
		if shared {
			nm.Shared()
		}
		a, _ := p.NewContext()
		b, _ := p.NewContext()
		if got := runIn(t, a, 0, `import { config, twice } from "host"; (config as any).level = 5; (config as any).level + twice(1)`); got != "7" {
			t.Fatalf("shared=%v a: got %s", shared, got)
		}
		want := "1"
		if shared {
			want = "5"
		}
		if got := runIn(t, b, 1, `import { config } from "host"; String((config as any).level)`); got != want {
			t.Fatalf("shared=%v b sees level %s, want %s", shared, got, want)
		}
		if shared && builds != 1 {
			t.Fatalf("shared module built %d times, want 1", builds)
		}
		if !shared && builds > 3 {
			t.Fatalf("per-realm module built %d times, want at most one per realm", builds)
		}
		p.Cleanup()
	}
}

// A native module imported only by another module (not by the entry) is
// available in every Context too.
func TestContextModulesNativeBehindModule(t *testing.T) {
	p := newModuleSession(t, map[string]string{
		"wrap": `import { twice } from "host"; export const four = (): number => twice(2);`,
	})
	p.DeclareModule("host", func(m *ModuleBuilder) {
		m.Function("twice", func(n int) int { return n * 2 })
		m.Default(nil)
	})
	for i := 0; i < 3; i++ {
		c, _ := p.NewContext()
		if got := runIn(t, c, i, `import { four } from "wrap"; four()`); got != "4" {
			t.Fatalf("context %d: got %s, want 4", i, got)
		}
	}
}

// Functions are per realm: properties and prototypes set on an exported
// function in one Context are not there in another, nor are intrinsics a
// script modified.
func TestContextModulesFunctionObjectsAndIntrinsicsIsolated(t *testing.T) {
	p := newModuleSession(t, map[string]string{
		"fn": `export function F() {} export const arrow = () => 1; export class K { m() { return 1; } }`,
	})
	a, _ := p.NewContext()
	b, _ := p.NewContext()
	runIn(t, a, 0, `import { F, arrow, K } from "fn";
(F.prototype as any).mark = 1; (F as any).tag = "a"; (arrow as any).tag = "a"; ((K as any).prototype).m = () => 2;
(Array.prototype as any).leak = 1; (Object.prototype as any).leak2 = 1;`)
	got := runIn(t, b, 1, `import { F, arrow, K } from "fn";
[(F.prototype as any).mark, (F as any).tag, (arrow as any).tag, new K().m(), (([]) as any).leak, ({} as any).leak2, F instanceof Function, Object.getPrototypeOf(F) === Function.prototype].join(",")`)
	if got != ",,,1,,,true,true" {
		t.Fatalf("b sees a's changes: %q", got)
	}
}
