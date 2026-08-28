package driver

import (
	"testing"

	"github.com/nooga/paserati/pkg/modules"
)

// Embed-API contract for noderati (issue #77 / docs/noderati-host-plan.md).
// A host outside cmd/paserati must be able to add resolvers, alias node:*
// specifiers, and default-import native modules without touching unexported fields.

func TestHostAddResolverBareSpecifier(t *testing.T) {
	p := NewPaserati()
	if p.GetModuleLoader() == nil {
		t.Fatal("GetModuleLoader returned nil")
	}

	mem := modules.NewMemoryResolver("bare-modules")
	mem.AddModule("left-pad", `
		export function pad(s: string): string {
			return "xx" + s;
		}
	`)
	p.AddResolver(mem)

	ts := `
		import { pad } from "left-pad";
		pad("hi");
	`
	result, errs := p.RunCode(ts, RunOptions{})
	if len(errs) > 0 {
		t.Fatalf("bare specifier via AddResolver failed: %v", errs[0])
	}
	if result.ToString() != "xxhi" {
		t.Errorf("expected xxhi, got %v", result.ToString())
	}
}

func TestHostDeclareModuleAliasSameRecord(t *testing.T) {
	p := NewPaserati()
	p.DeclareModule("fs", func(m *ModuleBuilder) {
		m.Function("readFileSync", func(path string) string {
			return "contents:" + path
		})
		m.Default(nil)
	})
	if err := p.DeclareModuleAlias("node:fs", "fs"); err != nil {
		t.Fatalf("DeclareModuleAlias: %v", err)
	}

	a, err := p.LoadModule("fs", ".")
	if err != nil {
		t.Fatalf("LoadModule(fs): %v", err)
	}
	b, err := p.LoadModule("node:fs", ".")
	if err != nil {
		t.Fatalf("LoadModule(node:fs): %v", err)
	}
	if a != b {
		t.Fatal("fs and node:fs must be the same module record")
	}

	fsRecord := p.GetModuleLoader().GetModule("fs")
	nodeRecord := p.GetModuleLoader().GetModule("node:fs")
	if fsRecord == nil || nodeRecord == nil {
		t.Fatal("expected both specifiers to be cached")
	}
	if fsRecord != nodeRecord {
		t.Fatal("cached fs and node:fs records differ")
	}
	if fsRecord.ResolvedPath != "native://fs" {
		t.Errorf("resolved path = %q, want native://fs", fsRecord.ResolvedPath)
	}
}

func TestHostNativeDefaultNamespaceImport(t *testing.T) {
	p := NewPaserati()
	p.DeclareModule("fs", func(m *ModuleBuilder) {
		m.Function("readFileSync", func(path string) string {
			return "contents:" + path
		})
		m.Default(nil)
	})
	if err := p.DeclareModuleAlias("node:fs", "fs"); err != nil {
		t.Fatalf("DeclareModuleAlias: %v", err)
	}

	ts := `
		import { readFileSync } from "fs";
		import fs from "node:fs";
		readFileSync("a.txt") + "|" + fs.readFileSync("b.txt");
	`
	result, errs := p.RunCode(ts, RunOptions{})
	if len(errs) > 0 {
		t.Fatalf("default/named import of aliased native module failed: %v", errs[0])
	}
	if result.ToString() != "contents:a.txt|contents:b.txt" {
		t.Errorf("got %q", result.ToString())
	}
}

func TestHostNativeDefaultFunctionExport(t *testing.T) {
	p := NewPaserati()
	p.DeclareModule("greet", func(m *ModuleBuilder) {
		m.Default(func(name string) string {
			return "hello " + name
		})
	})

	ts := `
		import greet from "greet";
		greet("noderati");
	`
	result, errs := p.RunCode(ts, RunOptions{})
	if len(errs) > 0 {
		t.Fatalf("default function export failed: %v", errs[0])
	}
	if result.ToString() != "hello noderati" {
		t.Errorf("got %q", result.ToString())
	}
}

func TestHostDeclareModuleAliasErrors(t *testing.T) {
	p := NewPaserati()
	if err := p.DeclareModuleAlias("node:fs", "fs"); err == nil {
		t.Fatal("expected error when aliasing a module that was never declared")
	}

	p.DeclareModule("fs", func(m *ModuleBuilder) {})
	p.DeclareModule("path", func(m *ModuleBuilder) {})
	if err := p.DeclareModuleAlias("node:fs", "fs"); err != nil {
		t.Fatalf("first alias should succeed: %v", err)
	}
	if err := p.DeclareModuleAlias("node:fs", "path"); err == nil {
		t.Fatal("expected error when alias already points at a different module")
	}
	if err := p.DeclareModuleAlias("node:fs", "fs"); err != nil {
		t.Fatalf("re-aliasing to the same module should be idempotent: %v", err)
	}
}
