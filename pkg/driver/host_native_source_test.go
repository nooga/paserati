package driver

import (
	"io"
	"strings"
	"testing"
)

func TestNativeModuleSyntheticSourceAfterLoad(t *testing.T) {
	p := NewPaserati()
	mod := p.DeclareModule("fs", func(m *ModuleBuilder) {
		m.Function("readFileSync", func(path string) string { return path })
		m.Const("sep", "/")
		m.Default(nil)
	})

	_, err := p.LoadModule("fs", ".")
	if err != nil {
		t.Fatalf("LoadModule(fs): %v", err)
	}

	data, err := io.ReadAll(&NativeModuleSource{module: mod, isNativeModule: true})
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	src := string(data)

	if strings.Contains(src, "PI_SQUARED") {
		t.Errorf("synthetic source must not contain PI_SQUARED placeholder:\n%s", src)
	}
	if !strings.Contains(src, "readFileSync") {
		t.Errorf("expected readFileSync in synthetic source:\n%s", src)
	}
	if !strings.Contains(src, "sep") {
		t.Errorf("expected sep in synthetic source:\n%s", src)
	}
	if !strings.Contains(src, "// Auto-generated native module: fs") {
		t.Errorf("expected header comment:\n%s", src)
	}
}

func TestNativeModuleSyntheticSourceBeforeLoad(t *testing.T) {
	p := NewPaserati()
	mod := p.DeclareModule("fs", func(m *ModuleBuilder) {
		m.Function("readFileSync", func(path string) string { return path })
		m.Const("sep", "/")
	})

	data, err := io.ReadAll(&NativeModuleSource{module: mod, isNativeModule: true})
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	src := string(data)

	if strings.Contains(src, "PI_SQUARED") {
		t.Errorf("synthetic source must not contain PI_SQUARED placeholder:\n%s", src)
	}
	if !strings.Contains(src, "export {}") {
		t.Errorf("expected export {} before load:\n%s", src)
	}
	if strings.Contains(src, "readFileSync") {
		t.Errorf("must not contain exports before load:\n%s", src)
	}
}
