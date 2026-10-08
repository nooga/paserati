package driver

import (
	"fmt"
	"math"
	"strings"
	"testing"

	"github.com/nooga/paserati/pkg/modules"
	"github.com/nooga/paserati/pkg/vm"
)

// TestNativeModuleBasic tests the basic native module declaration and usage
func TestNativeModuleBasic(t *testing.T) {
	// Create a new Paserati instance
	p := NewPaserati()

	// Declare a simple math utilities module
	p.DeclareModule("math-utils", func(m *ModuleBuilder) {

		// Add some constants
		m.Const("PI_SQUARED", math.Pi*math.Pi)
		m.Const("EULER", math.E)

		// Add simple functions
		m.Function("square", func(x float64) float64 {
			return x * x
		})

		m.Function("add", func(a, b float64) float64 {
			return a + b
		})

		// Add a function that returns multiple values (should return tuple or object)
		m.Function("divmod", func(a, b float64) map[string]float64 {
			return map[string]float64{
				"quotient":  math.Floor(a / b),
				"remainder": math.Mod(a, b),
			}
		})
	})

	// NOTE: TypeScript code for later integration test
	_ = `
		// Import from our native module
		import { square, add, divmod, PI_SQUARED, EULER } from "math-utils";
		
		console.log("Testing native module imports...");
		
		// Test constants
		console.log("PI_SQUARED =", PI_SQUARED);
		console.log("EULER =", EULER);
		
		// Test simple functions
		let result1 = square(5);
		console.log("square(5) =", result1);
		
		let result2 = add(10, 20);
		console.log("add(10, 20) =", result2);
		
		// Test complex return values
		let result3 = divmod(17, 5);
		console.log("divmod(17, 5) =", result3);
		console.log("quotient =", result3.quotient);
		console.log("remainder =", result3.remainder);
		
		// Return success indicator
		"native_module_test_passed";
	`

	// Test that the native module can be resolved
	moduleRecord := p.moduleLoader.GetModule("math-utils", ".")
	if moduleRecord == nil {
		// Let's try resolving it directly
		if p.nativeResolver != nil {
			nativeResolver := p.nativeResolver
			canResolve := nativeResolver.CanResolve("math-utils")

			if canResolve {
				// Try to manually load the module to see what happens
				loadedModule, err := p.moduleLoader.LoadModule("math-utils", ".")
				if err == nil && loadedModule != nil {
					if concreteRecord, ok := loadedModule.(*modules.ModuleRecord); ok {
						moduleRecord = concreteRecord
					}
				}
			}
		}

		if moduleRecord == nil {
			t.Fatalf("Native module 'math-utils' was not registered with module loader")
		}
	}

	// For now, let's just test that the native module is properly registered
	// and that we can access the native resolver
	nativeResolver := p.nativeResolver
	if !nativeResolver.CanResolve("math-utils") {
		t.Fatalf("Native resolver cannot resolve 'math-utils'")
	}

	// Test resolving the module
	_, err := nativeResolver.Resolve("math-utils", ".")
	if err != nil {
		t.Fatalf("Failed to resolve native module: %v", err)
	}

	// Test actual TypeScript execution with imports

	tsCode := `
		import { square, add, divmod, PI_SQUARED, EULER } from "math-utils";
		
		console.log("Testing native module imports...");
		
		// Test constants
		console.log("PI_SQUARED =", PI_SQUARED);
		console.log("EULER =", EULER);
		
		// Test simple functions
		let result1 = square(5);
		console.log("square(5) =", result1);
		
		let result2 = add(10, 20);
		console.log("add(10, 20) =", result2);
		
		// Test complex return values
		let result3 = divmod(17, 5);
		console.log("divmod(17, 5) =", result3);
		console.log("quotient =", result3.quotient);
		console.log("remainder =", result3.remainder);
		
		// Return success indicator
		"native_module_test_passed";
	`

	result, errs := p.RunStringWithModules(tsCode)
	if len(errs) > 0 {
		t.Fatalf("Failed to evaluate native module test: %v", errs[0])
	}

	// Check that we got the expected result
	if result.ToString() != "native_module_test_passed" {
		t.Errorf("Expected 'native_module_test_passed', got: %v", result.ToString())
	}
}

// TestNativeModuleNamespace tests namespace functionality
func TestNativeModuleNamespace(t *testing.T) {
	p := NewPaserati()

	// Declare a module with namespaces
	p.DeclareModule("collections", func(m *ModuleBuilder) {
		// Math namespace
		m.Namespace("math", func(ns *NamespaceBuilder) {
			ns.Function("abs", math.Abs)
			ns.Function("sqrt", math.Sqrt)
			ns.Const("PI", math.Pi)
		})

		// String utilities namespace
		m.Namespace("strings", func(ns *NamespaceBuilder) {
			ns.Function("upper", func(s string) string {
				return strings.ToUpper(s)
			})
			ns.Function("lower", func(s string) string {
				return strings.ToLower(s)
			})
		})
	})

	tsCode := `
		import { math, strings } from "collections";
		
		console.log("Testing namespaced functions...");
		
		let absResult = math.abs(-42);
		console.log("math.abs(-42) =", absResult);
		
		let sqrtResult = math.sqrt(16);
		console.log("math.sqrt(16) =", sqrtResult);
		
		let upperResult = strings.upper("hello");
		console.log("strings.upper('hello') =", upperResult);
		
		"namespace_test_passed";
	`

	result, errs := p.RunStringWithModules(tsCode)
	if len(errs) > 0 {
		t.Fatalf("Failed to evaluate namespace test: %v", errs[0])
	}

	if result.ToString() != "namespace_test_passed" {
		t.Errorf("Expected 'namespace_test_passed', got: %v", result.ToString())
	}
}

// Simple Point struct for testing with JSON tags
type Point struct {
	X      float64 `json:"x"`           // Mapped to "x"
	Y      float64 `json:"y"`           // Mapped to "y"
	Z      float64 `json:"z,omitempty"` // Mapped to "z" with omitempty (not used yet)
	Name   string  `json:"name"`        // Mapped to "name"
	ID     int     `json:"id"`          // Mapped to "id"
	Public float64 // No tag, uses field name "Public"
	// Hidden field that won't be exposed to TypeScript
	internal string `json:"-"`
}

// Method for Point
func (p *Point) Distance() float64 {
	return math.Sqrt(p.X*p.X + p.Y*p.Y)
}

func (p *Point) String() string {
	return fmt.Sprintf("Point(%f, %f)", p.X, p.Y)
}

// TestNativeModuleClass tests class/struct functionality
func TestNativeModuleClass(t *testing.T) {
	p := NewPaserati()

	p.DeclareModule("geometry", func(m *ModuleBuilder) {
		// Export Point class with constructor
		m.Class("Point", (*Point)(nil), func(x, y float64) *Point {
			return &Point{
				X: x, Y: y, Z: 0,
				Name: "TestPoint", ID: 42, Public: 99.5,
				internal: "hidden", // This should not be accessible from TypeScript
			}
		})
	})

	tsCode := `
		import { Point } from "geometry";
		
		console.log("Testing class functionality...");
		
		let p1 = new Point(3, 4);
		console.log("Created point:", p1);
		
		// Test JSON tag mapping: Go fields should be accessible with their JSON names
		console.log("Point x coordinate:", p1.x);        // X -> x
		console.log("Point y coordinate:", p1.y);        // Y -> y
		console.log("Point z coordinate:", p1.z);        // Z -> z
		console.log("Point name:", p1.name);             // Name -> name
		console.log("Point ID:", p1.id);                 // ID -> id
		console.log("Point Public field:", p1.Public);   // No tag, uses field name
		
		// Verify the internal field is not accessible (should be undefined)
		console.log("Internal field (should be undefined):", p1.internal);
		
		let distance = p1.distance();
		console.log("Distance from origin:", distance);
		
		"class_test_passed";
	`

	result, errs := p.RunStringWithModules(tsCode)
	if len(errs) > 0 {
		t.Fatalf("Failed to evaluate class test: %v", errs[0])
	}

	if result.ToString() != "class_test_passed" {
		t.Errorf("Expected 'class_test_passed', got: %v", result.ToString())
	}
}

// TestNativeModuleClassInstanceof is a regression test for paserati#201:
// `ModuleBuilder.Class`-registered constructors threw
// "Function has non-object prototype in instanceof check" on `instanceof`
// instead of the instance's prototype chain actually including the
// constructor's `.prototype`, because createClassConstructor built its
// constructor with no Properties table (and so nowhere to put one) and gave
// created instances Undefined as their [[Prototype]].
func TestNativeModuleClassInstanceof(t *testing.T) {
	p := NewPaserati()

	p.DeclareModule("geometry2", func(m *ModuleBuilder) {
		m.Class("Point", (*Point)(nil), func(x, y float64) *Point {
			return &Point{X: x, Y: y}
		})
	})

	tsCode := `
		import { Point } from "geometry2";

		let p1 = new Point(3, 4);

		let checks: any[] = [
			p1 instanceof Point,
			(new Point(0, 0)) instanceof Point,
			typeof Point.prototype === "object",
			p1.constructor === Point,
			(({} as any) instanceof Point) === false,
			// 'constructor' lives on the shared prototype as a non-enumerable
			// property, same as FormData's - it must not leak into
			// enumeration or serialization of an instance.
			JSON.stringify(p1).indexOf("constructor") === -1,
			Object.keys(p1).indexOf("constructor") === -1,
			// Instances now chain to Object.prototype (previously
			// Undefined), so ordinary Object.prototype methods resolve
			// instead of being undefined.
			typeof p1.hasOwnProperty === "function",
			p1.hasOwnProperty("x") === true,
			p1.hasOwnProperty("constructor") === false,
			// The constructor's own .name should be the class name given to
			// Class(), not the generic internal placeholder it used to be.
			(Point as any).name === "Point",
			p1.constructor.name === "Point",
		];

		checks.every((c) => c === true) ? "instanceof_test_passed" : "FAIL: " + JSON.stringify(checks);
	`

	result, errs := p.RunStringWithModules(tsCode)
	if len(errs) > 0 {
		t.Fatalf("Failed to evaluate instanceof test: %v", errs[0])
	}

	if result.ToString() != "instanceof_test_passed" {
		t.Errorf("Expected 'instanceof_test_passed', got: %v", result.ToString())
	}
}

// Go signatures map to TS signatures callers can use as written: variadic
// funcs take rest arguments, trailing pointer parameters and omitempty or
// pointer fields are optional, a pointer result can be null, and a lone
// error result throws.
func TestNativeModuleGoSignatureTypes(t *testing.T) {
	type opts struct {
		Method  string            `json:"method,omitempty"`
		Headers map[string]string `json:"headers,omitempty"`
		Timeout *int              `json:"timeout"`
	}
	type resp struct {
		Status int    `json:"status"`
		Body   string `json:"body"`
		Error  string `json:"error,omitempty"`
	}
	declare := func(p *Paserati) {
		p.DeclareModule("host", func(m *ModuleBuilder) {
			m.Function("output", func(args ...vm.Value) int { return len(args) })
			m.Function("join", func(sep string, parts ...string) string { return strings.Join(parts, sep) })
			m.Function("get", func(url string, o *opts) resp {
				if o == nil {
					return resp{Status: 200, Body: url}
				}
				return resp{Status: 201, Body: o.Method + " " + url + " " + o.Headers["a"]}
			})
			m.Function("lookup", func(k string) *string {
				if k == "" {
					return nil
				}
				return &k
			})
			m.Function("find", func(k string) (*resp, error) {
				if k == "" {
					return nil, fmt.Errorf("no key")
				}
				return &resp{Status: 1, Body: k}, nil
			})
			m.Function("fail", func(msg string) error {
				if msg == "" {
					return nil
				}
				return fmt.Errorf("%s", msg)
			})
			m.Type("Resp", resp{})
			m.Default(nil)
		})
	}
	for _, tc := range []struct{ name, src, want string }{
		{"rest", `import h from "host"; h.output() + h.output("a") + h.output("a", 1, {x: 1})`, "4"},
		{"typed rest", `import { join } from "host"; join("-") + join("-", "a", "b")`, "a-b"},
		{"optional pointer param", `import h from "host"; const r = h.get("u"); const n: number = r.status; n + r.body`, "200u"},
		{"omitempty fields", `import h from "host"; h.get("u", { headers: { a: "b" } }).body`, " u b"},
		{"pointer field", `import h from "host"; h.get("u", { method: "GET", timeout: 5 }).status`, "201"},
		{"optional field in type", `import { Resp } from "host"; const r: Resp = { status: 1, body: "" }; r.status`, "1"},
		{"nullable result", `import h from "host"; const v = h.lookup(""); v === null ? "null" : v.length`, "null"},
		{"(*T, error) is not nullable", `import h from "host"; h.find("k").body`, "k"},
		{"lone error throws", `import h from "host"; let out = String(h.fail("")); try { h.fail("boom"); } catch (e) { out += " " + (e as Error).message; } out`, "undefined boom"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := NewPaserati()
			defer p.Cleanup()
			declare(p)
			v, errs := p.RunCode(tc.src, RunOptions{ModuleName: "/main.ts", Filename: "/main.ts"})
			if len(errs) > 0 {
				t.Fatalf("errors: %v", errs)
			}
			if got := v.ToString(); got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}

	// Still checked: a required parameter, a rest element type and a nullable
	// result are enforced.
	for _, tc := range []struct{ name, src string }{
		{"missing required", `import h from "host"; h.get();`},
		{"rest element type", `import { join } from "host"; join("-", 1);`},
		{"nullable result", `import h from "host"; const n: number = h.lookup("k").length;`},
	} {
		t.Run("rejects "+tc.name, func(t *testing.T) {
			p := NewPaserati()
			defer p.Cleanup()
			declare(p)
			if _, errs := p.RunCode(tc.src, RunOptions{ModuleName: "/main.ts", Filename: "/main.ts"}); len(errs) == 0 {
				t.Fatal("expected a type error")
			}
		})
	}
}
