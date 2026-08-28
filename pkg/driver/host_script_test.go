package driver

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/nooga/paserati/pkg/vm"
)

func wrapCJS(body string) string {
	return "(function (exports, require, module, __filename, __dirname) {\n" + body + "\n})"
}

func TestHostRunScriptCJSWrap(t *testing.T) {
	p := NewPaserati()
	p.SetSkipTypeCheck(true)

	filename := "/app/lib/foo.js"
	dirname := filepath.Dir(filename)
	body := `module.exports = { file: __filename, dir: __dirname, n: 1 + exports.seed };`
	fn, errs := p.RunScript(wrapCJS(body), filename)
	if len(errs) > 0 {
		t.Fatalf("RunScript failed: %v", errs[0])
	}
	if !fn.IsCallable() {
		t.Fatalf("expected wrapper function, got %s", fn.ToString())
	}

	vmInst := p.GetVM()
	exportsVal := vm.NewObject(vmInst.ObjectPrototype)
	exportsVal.AsPlainObject().SetOwn("seed", vm.NumberValue(41))
	moduleVal := vm.NewObject(vmInst.ObjectPrototype)
	moduleVal.AsPlainObject().SetOwn("exports", exportsVal)

	_, err := vmInst.Call(fn, vm.Undefined, []vm.Value{
		exportsVal,
		vm.Undefined,
		moduleVal,
		vm.String(filename),
		vm.String(dirname),
	})
	if err != nil {
		t.Fatalf("CJS wrapper call failed: %v", err)
	}

	gotExports, ok := moduleVal.AsPlainObject().GetOwn("exports")
	if !ok || !gotExports.IsObject() {
		t.Fatal("module.exports missing after wrapper call")
	}
	file, ok := gotExports.AsPlainObject().GetOwn("file")
	if !ok || file.ToString() != filename {
		t.Errorf("__filename = %q, want %q", file.ToString(), filename)
	}
	dir, ok := gotExports.AsPlainObject().GetOwn("dir")
	if !ok || dir.ToString() != dirname {
		t.Errorf("__dirname = %q, want %q", dir.ToString(), dirname)
	}
	n, ok := gotExports.AsPlainObject().GetOwn("n")
	if !ok || n.ToFloat() != 42 {
		t.Errorf("n = %v, want 42", n.ToString())
	}
}

func TestHostRunScriptNoModuleRecord(t *testing.T) {
	p := NewPaserati()
	p.SetSkipTypeCheck(true)

	filename := "/tmp/cjs-not-esm.js"
	_, errs := p.RunScript("var x = 1; x", filename)
	if len(errs) > 0 {
		t.Fatalf("RunScript failed: %v", errs[0])
	}
	if rec := p.GetModuleLoader().GetModule(filename); rec != nil {
		t.Fatal("RunScript must not invent a ModuleRecord")
	}
}

func TestHostRunScriptRejectsImportMeta(t *testing.T) {
	p := NewPaserati()
	p.SetSkipTypeCheck(true)

	_, errs := p.RunCode("export const a = 1; a", RunOptions{})
	if len(errs) > 0 {
		t.Fatalf("module RunCode failed: %v", errs[0])
	}

	_, errs = p.RunScript("import.meta.url", "/app/lib/foo.js")
	if len(errs) == 0 {
		t.Fatal("expected import.meta to fail in script mode")
	}
	if !strings.Contains(errs[0].Error(), "import.meta") {
		t.Errorf("error = %q, want import.meta", errs[0].Error())
	}
}

func TestHostRunOptionsScript(t *testing.T) {
	p := NewPaserati()
	p.SetSkipTypeCheck(true)

	result, errs := p.RunCode("1 + 2", RunOptions{Script: true, Filename: "/tmp/add.js"})
	if len(errs) > 0 {
		t.Fatalf("RunCode Script failed: %v", errs[0])
	}
	if result.ToFloat() != 3 {
		t.Errorf("got %v, want 3", result.ToString())
	}
}
