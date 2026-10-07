package tests

import (
	"strings"
	"testing"

	"github.com/nooga/paserati/pkg/driver"
	"github.com/nooga/paserati/pkg/vm"
)

// #594: a top-level uncaught exception must not poison the instance.
func TestInstanceReusableAfterUncaughtThrow(t *testing.T) {
	for _, module := range []bool{false, true} {
		p := driver.NewPaserati()
		p.SetSkipTypeCheck(true)
		calls := 0
		p.GetVM().GlobalObject.SetOwn("tick", vm.NewNativeFunction(0, false, "tick",
			func(a []vm.Value) (vm.Value, error) { calls++; return vm.Undefined, nil }))
		opts := driver.RunOptions{Script: !module, Filename: "x.js"}

		run := func(src string) (vm.Value, string) {
			v, errs := p.RunCode(src, opts)
			if len(errs) > 0 {
				return v, errs[0].Error()
			}
			return v, ""
		}

		if v, e := run("tick(); 1"); e != "" || v.ToString() != "1" {
			t.Fatalf("module=%v initial: %v %q", module, v.ToString(), e)
		}
		for i, msg := range []string{"first", "second", "third"} {
			if _, e := run("tick(); throw new Error('" + msg + "')"); !strings.Contains(e, msg) {
				t.Fatalf("module=%v throw %d: want error mentioning %q, got %q", module, i, msg, e)
			}
			before := calls
			v, e := run("tick(); 40 + 2")
			if e != "" || v.ToString() != "42" || calls != before+1 {
				t.Fatalf("module=%v after throw %d: v=%v err=%q ran=%v", module, i, v.ToString(), e, calls == before+1)
			}
		}
		p.GetVM().Reset()
		if v, e := run("2+2"); e != "" || v.ToString() != "4" {
			t.Fatalf("module=%v after Reset: %v %q", module, v.ToString(), e)
		}
	}
}
