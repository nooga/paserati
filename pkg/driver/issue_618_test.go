package driver

import (
	"math/big"
	"testing"

	"github.com/nooga/paserati/pkg/vm"
)

type resp618 struct {
	StatusCode int    `json:"statusCode"`
	Body       string `json:"body"`
}

// ModuleBuilder conversions handle the common Go shapes: interface{}
// values, JSON-like maps and slices, structs by value, slice and func
// parameters (#618).
func TestModuleBuilderConversions(t *testing.T) {
	p := NewPaserati()
	p.SetSkipTypeCheck(true)
	p.DeclareModule("host", func(m *ModuleBuilder) {
		m.Function("mapIface", func() map[string]interface{} {
			return map[string]interface{}{"a": 1, "b": "x", "c": []interface{}{1, 2}}
		})
		m.Function("mapFloat", func() map[string]float64 { return map[string]float64{"a": 1} })
		m.Function("ifaceRet", func() interface{} { return map[string]interface{}{"k": 1} })
		m.Function("nilIface", func() interface{} { return nil })
		m.Function("structVal", func() resp618 { return resp618{200, "ok"} })
		m.Function("structPtr", func() *resp618 { return &resp618{200, "ok"} })
		m.Function("sum", func(xs []float64) float64 {
			s := 0.0
			for _, x := range xs {
				s += x
			}
			return s
		})
		m.Function("count", func(xs []interface{}) int { return len(xs) })
		m.Function("kinds", func(xs []interface{}) string {
			out := ""
			for _, x := range xs {
				switch x.(type) {
				case float64:
					out += "n"
				case string:
					out += "s"
				case bool:
					out += "b"
				case []interface{}:
					out += "a"
				case map[string]interface{}:
					out += "o"
				case nil:
					out += "_"
				}
			}
			return out
		})
		m.Function("takesMap", func(o map[string]interface{}) int { return len(o) })
		m.Function("apply", func(f func(float64) float64, x float64) float64 { return f(x) })
		m.Function("applyErr", func(f func(float64) (float64, error), x float64) string {
			_, err := f(x)
			if err != nil {
				return "err"
			}
			return "ok"
		})
	})
	cases := map[string]string{
		`JSON.stringify(mapIface())`:                                   `{"a":1,"b":"x","c":[1,2]}`,
		`JSON.stringify(mapFloat())`:                                   `{"a":1}`,
		`JSON.stringify(ifaceRet())`:                                   `{"k":1}`,
		`String(nilIface())`:                                           `null`,
		`JSON.stringify(structVal())`:                                  `{"statusCode":200,"body":"ok"}`,
		`JSON.stringify(structPtr())`:                                  `{"statusCode":200,"body":"ok"}`,
		`String(sum([1, 2, 3]))`:                                       `6`,
		`String(count([1, "a", true]))`:                                `3`,
		`kinds([1, "a", true, [1], {x: 1}, null])`:                     `nsbao_`,
		`String(takesMap({a: 1, b: 2}))`:                               `2`,
		`String(apply(x => x * 2, 21))`:                                `42`,
		`applyErr(x => { throw new Error("no") }, 1)`:                  `err`,
		`String(mapIface().hasOwnProperty("a"))`:                       `true`,
		`String(Object.getPrototypeOf(ifaceRet()) === Object.prototype)`: `true`,
	}
	for src, want := range cases {
		v, errs := p.RunCode(`import { mapIface, mapFloat, ifaceRet, nilIface, structVal, structPtr, sum, count, kinds, takesMap, apply, applyErr } from "host"; `+src, RunOptions{})
		if len(errs) > 0 {
			t.Errorf("%s: %v", src, errs[0])
			continue
		}
		if got := v.ToString(); got != want {
			t.Errorf("%s = %s, want %s", src, got, want)
		}
	}
}

func TestToValueExport(t *testing.T) {
	p := NewPaserati()
	v := p.ToValue(map[string]interface{}{"a": []interface{}{1, "x", nil}, "b": resp618{1, "y"}})
	fn, errs := p.RunCode(`(v) => JSON.stringify(v)`, RunOptions{Script: true})
	if len(errs) > 0 {
		t.Fatal(errs[0])
	}
	got, err := p.GetVM().Call(fn, vm.Undefined, []vm.Value{v})
	if err != nil {
		t.Fatal(err)
	}
	if got.ToString() != `{"a":[1,"x",null],"b":{"statusCode":1,"body":"y"}}` {
		t.Fatalf("got %s", got.ToString())
	}
	back, errs := p.RunCode(`({n: 1, s: "t", l: [true, null], o: {k: 2n}})`, RunOptions{Script: true})
	if len(errs) > 0 {
		t.Fatal(errs[0])
	}
	m, ok := p.Export(back).(map[string]interface{})
	if !ok || m["n"] != 1.0 || m["s"] != "t" || len(m["l"].([]interface{})) != 2 || m["o"].(map[string]interface{})["k"].(*big.Int).Int64() != 2 {
		t.Fatalf("Export = %#v", p.Export(back))
	}
}
