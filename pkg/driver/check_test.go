package driver

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nooga/paserati/pkg/nodemodules"
)

func TestCheckFiles(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		// A typed package: its types reach the checker, its body never runs.
		"node_modules/pkg/package.json": `{"exports":{".":{"types":"./index.d.ts","import":"./index.js"}}}`,
		"node_modules/pkg/index.d.ts":   "export declare function twice(n: number): number;\nexport declare const broken: Missing;\n",
		"node_modules/pkg/index.js":     "throw new Error('must not run');\n",
		"lib.ts":                        "export const base: number = 1;\n",
		"main.ts":                       "import { twice } from \"pkg\";\nimport { base } from \"./lib.ts\";\nconst s: string = twice(base);\n",
	}
	for p, content := range files {
		full := filepath.Join(root, filepath.FromSlash(p))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	wd, _ := os.Getwd()
	defer os.Chdir(wd)
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}

	p := NewPaserati()
	p.AddResolver(nodemodules.NewResolver(nodemodules.Options{Mode: nodemodules.Types}))
	results := p.CheckFiles([]string{"main.ts"})

	byFile := map[string][]string{}
	for _, r := range results {
		if r.LoadError != nil {
			t.Fatalf("%s: load error: %v", r.Path, r.LoadError)
		}
		rel, _ := filepath.Rel(root, r.Path)
		for _, d := range r.Diagnostics {
			byFile[rel] = append(byFile[rel], d.Code()+" "+d.Message())
		}
		if _, ok := byFile[rel]; !ok {
			byFile[rel] = nil
		}
	}
	if _, ok := byFile["lib.ts"]; !ok {
		t.Errorf("lib.ts (imported) was not checked: %v", byFile)
	}
	for f := range byFile {
		if strings.Contains(f, "node_modules") {
			t.Errorf("library file %s reported (skipLibCheck)", f)
		}
	}
	got := byFile["main.ts"]
	if len(got) != 1 || !strings.Contains(got[0], "TS2322") || !strings.Contains(got[0], "'number' is not assignable to type 'string'") {
		t.Errorf("main.ts diagnostics = %q, want one TS2322 number-to-string", got)
	}
}

func TestCheckFilesReportsSyntaxErrors(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "bad.ts"), []byte("const x = ;\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	wd, _ := os.Getwd()
	defer os.Chdir(wd)
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	results := NewPaserati().CheckFiles([]string{"bad.ts"})
	var got []string
	for _, r := range results {
		for _, d := range r.Diagnostics {
			got = append(got, d.Code())
		}
	}
	if len(got) != 1 || got[0] != "TS1109" {
		t.Errorf("diagnostics = %v, want [TS1109]", got)
	}
}

// TestCheckFilesGlobalDeclarations checks @types/node's web-globals pattern:
// a module-local interface feeds a `declare global` interface through a
// `typeof globalThis` conditional, and a global extended by one file
// (EventTarget) is declared by a file loaded after it.
func TestCheckFilesGlobalDeclarations(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		"node_modules/@types/web/package.json": `{"types":"index.d.ts"}`,
		"node_modules/@types/web/index.d.ts":   "/// <reference path=\"abort.d.ts\" />\n/// <reference path=\"events.d.ts\" />\n",
		"node_modules/@types/web/abort.d.ts": "export {};\n" +
			"type _AbortSignal = typeof globalThis extends { onmessage: any } ? {} : AbortSignal;\n" +
			"interface AbortSignal extends EventTarget { readonly aborted: boolean; }\n" +
			"declare global { interface AbortSignal extends _AbortSignal {} }\n",
		"node_modules/@types/web/events.d.ts": "export {};\n" +
			"type _EventTarget = typeof globalThis extends { onmessage: any } ? {} : EventTarget;\n" +
			"interface EventTarget { addEventListener(type: string, listener: () => void): void; }\n" +
			"declare global { interface EventTarget extends _EventTarget {} }\n",
		"main.ts": "declare const s: AbortSignal;\n" +
			"s.addEventListener(\"abort\", () => {});\n" +
			"const a: boolean = s.aborted;\n" +
			"const bad: number = s.aborted;\n" +
			"export {};\n",
	}
	for p, content := range files {
		full := filepath.Join(root, filepath.FromSlash(p))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	wd, _ := os.Getwd()
	defer os.Chdir(wd)
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}

	p := NewPaserati()
	p.AddResolver(nodemodules.NewResolver(nodemodules.Options{Mode: nodemodules.Types}))
	opts := DefaultCheckOptions()
	opts.Types = []string{"web"}
	opts.TypesFrom = root
	var got []string
	for _, r := range p.CheckFilesWithOptions([]string{"main.ts"}, opts) {
		for _, d := range r.Diagnostics {
			got = append(got, d.Code()+" "+d.Message())
		}
	}
	want := "TS2322 Type 'boolean' is not assignable to type 'number'."
	if len(got) != 1 || got[0] != want {
		t.Errorf("diagnostics = %q, want only %q", got, want)
	}
}

// A global declared by a module loaded late (vitest's index.d.ts imports
// 'chai' after the chunk that maps over Chai.Assertion) is visible to the
// modules checked before it: tsc binds every file's globals first.
func TestCheckFilesGlobalDeclaredLater(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		"lib.d.ts": "type VA<A> = { [K in keyof A]: A[K] extends Chai.Assertion ? Asrt : A[K] };\n" +
			"export interface Asrt extends VA<Chai.Assertion> { toBe(v: any): void; }\n" +
			"export declare function expect(v: any): Asrt;\n",
		"chai.d.ts": "declare global { namespace Chai { interface Assertion { not: Assertion; } } }\nexport {};\n",
		"idx.d.ts":  "export * from './lib';\nimport './chai';\n",
		"main.ts":   "import { expect } from './idx';\nexpect(1).not.toBe(2);\nconst bad: number = expect(1).not;\nexport {};\n",
	}
	for p, content := range files {
		full := filepath.Join(root, filepath.FromSlash(p))
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	wd, _ := os.Getwd()
	defer os.Chdir(wd)
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}

	p := NewPaserati()
	var got []string
	for _, r := range p.CheckFilesWithOptions([]string{"main.ts"}, DefaultCheckOptions()) {
		if r.LoadError != nil {
			t.Fatalf("%s: load error: %v", r.Path, r.LoadError)
		}
		for _, d := range r.Diagnostics {
			got = append(got, d.Code()+" "+d.Message())
		}
	}
	want := "TS2322 Type 'Asrt' is not assignable to type 'number'."
	if len(got) != 1 || got[0] != want {
		t.Errorf("diagnostics = %q, want only %q", got, want)
	}
}

// A module augmentation merges into the module it names, as vitest adds its
// snapshot matchers to Assertion: from a chunk the augmented module itself
// imports (so the target is still being checked), through a re-export, and
// into instances made while the interface was declared (`rejects`).
func TestCheckFilesModuleAugmentation(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		"node_modules/testlib/package.json": `{"types":"index.d.ts"}`,
		"node_modules/testlib/index.d.ts": "export { Assertion, expect } from './core.js';\n" +
			"import './snapshot.js';\n",
		"node_modules/testlib/core.d.ts": "type PromisifyAssertion<T> = Assertion<Promise<void>, Awaited<T>>;\n" +
			"interface Assertion<R = void, T = unknown> { toBe(x: T): R; rejects: PromisifyAssertion<unknown>; }\n" +
			"declare function expect<T>(x: T): Assertion<void, T>;\n" +
			"export { Assertion, expect };\n",
		"node_modules/testlib/snapshot.d.ts": "export {};\n" +
			"declare module 'testlib' {\n" +
			"  interface Assertion<R, T> { toMatchInlineSnapshot(snapshot?: string): R; }\n" +
			"  interface Extra { z: boolean }\n" +
			"}\n",
		"main.ts": "import { expect, type Extra } from 'testlib';\n" +
			"expect(1).toMatchInlineSnapshot();\n" +
			"expect(Promise.resolve(1)).rejects.toMatchInlineSnapshot();\n" +
			"const e: Extra = { z: true };\n" +
			"const bad: number = expect(1).toMatchInlineSnapshot();\n",
	}
	for p, content := range files {
		full := filepath.Join(root, filepath.FromSlash(p))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	wd, _ := os.Getwd()
	defer os.Chdir(wd)
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}

	p := NewPaserati()
	p.AddResolver(nodemodules.NewResolver(nodemodules.Options{Mode: nodemodules.Types}))
	var got []string
	for _, r := range p.CheckFiles([]string{"main.ts"}) {
		for _, d := range r.Diagnostics {
			got = append(got, d.Code()+" "+d.Message())
		}
	}
	want := "TS2322 Type 'void' is not assignable to type 'number'."
	if len(got) != 1 || got[0] != want {
		t.Errorf("diagnostics = %q, want only %q", got, want)
	}
}

// TestCheckFilesJSX checks JSX against a React-shaped declaration package:
// `export = React` with the JSX namespace nested in it, an interface that a
// class of the same name merges into, a generic component inferred from its
// attributes, and tsc's codes for attribute mismatches.
func TestCheckFilesJSX(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		"node_modules/react/package.json": `{"types":"index.d.ts"}`,
		"node_modules/react/index.d.ts": "export = React;\n" +
			"declare namespace React {\n" +
			"  type ReactNode = string | number | boolean | null | undefined | JSX.Element;\n" +
			"  interface Attributes { key?: string | number }\n" +
			"  interface Component<P = {}, S = {}, SS = any> {}\n" +
			"  class Component<P, S> { constructor(props: P); readonly props: Readonly<P>; render(): ReactNode; }\n" +
			"  namespace JSX {\n" +
			"    interface Element { type: any }\n" +
			"    interface ElementAttributesProperty { props: {} }\n" +
			"    interface ElementChildrenAttribute { children: {} }\n" +
			"    interface IntrinsicAttributes extends React.Attributes {}\n" +
			"    interface IntrinsicElements { div: { className?: string; children?: ReactNode; onClick?: (e: { x: number }) => void } }\n" +
			"  }\n" +
			"}\n",
		"main.tsx": "import * as React from 'react';\n" +
			"function Hello(props: { name: string; children?: React.ReactNode }) { return <div className=\"h\">{props.name}</div>; }\n" +
			"class Klass extends React.Component<{ title: string }> { render() { return <div>{this.props.title}</div>; } }\n" +
			"function List<T>(props: { items: T[]; render: (item: T) => string }) { return <div />; }\n" +
			"const ok = <>\n" +
			"  <Hello name=\"a\" key=\"k\" data-x=\"1\">text {1}</Hello>\n" +
			"  <Klass title=\"t\" />\n" +
			"  <List items={[1, 2]} render={(n) => n.toFixed(1)} />\n" +
			"  <div onClick={(e) => e.x.toFixed()} />\n" +
			"</>;\n" +
			"const a = <Hello />;\n" +
			"const b = <Hello name={1} />;\n" +
			"const c = <Hello name=\"x\" extra />;\n" +
			"const d = <span />;\n",
	}
	for p, content := range files {
		full := filepath.Join(root, filepath.FromSlash(p))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	wd, _ := os.Getwd()
	defer os.Chdir(wd)
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}

	p := NewPaserati()
	p.AddResolver(nodemodules.NewResolver(nodemodules.Options{Mode: nodemodules.Types}))
	var got []string
	for _, r := range p.CheckFiles([]string{"main.tsx"}) {
		for _, d := range r.Diagnostics {
			got = append(got, fmt.Sprintf("%d:%d %s", d.Pos().Line, d.Pos().Column, d.Code()))
		}
	}
	want := []string{"11:12 TS2741", "12:18 TS2322", "13:27 TS2322", "14:12 TS2339"}
	if strings.Join(got, ", ") != strings.Join(want, ", ") {
		t.Errorf("diagnostics = %q, want %q", got, want)
	}
}

// Ambient module declarations in a types package (@types/node style): an
// import names the module a script file's `declare module "m"` declares,
// declarations of one module merge across files, a body's imports reach
// modules declared in files referenced after it, and export= of a namespace
// merged with a const is the const.
func TestCheckFilesAmbientModules(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		"node_modules/@types/node/package.json": `{"types":"index.d.ts"}`,
		"node_modules/@types/node/index.d.ts": "/// <reference path=\"events.d.ts\" />\n" +
			"/// <reference path=\"net.d.ts\" />\n" +
			"/// <reference path=\"path.d.ts\" />\n",
		"node_modules/@types/node/events.d.ts": "declare module \"events\" {\n" +
			"  import { Socket } from \"node:net\";\n" +
			"  class EventEmitter { on(e: string): this; sock: Socket; }\n" +
			"  export { EventEmitter };\n" +
			"}\n" +
			"declare module \"node:events\" { export * from \"events\"; }\n",
		"node_modules/@types/node/net.d.ts": "declare module \"net\" {\n" +
			"  class Socket { remote: string; }\n" +
			"}\n" +
			"declare module \"net\" { function connect(): Socket; }\n" +
			"declare module \"node:net\" { export * from \"net\"; }\n",
		"node_modules/@types/node/path.d.ts": "declare module \"path\" {\n" +
			"  namespace path { interface PlatformPath { join(...p: string[]): string } }\n" +
			"  const path: path.PlatformPath;\n" +
			"  export = path;\n" +
			"}\n",
		"main.ts": "import { EventEmitter } from 'node:events';\n" +
			"import { connect } from 'net';\n" +
			"import * as path from 'path';\n" +
			"const a: number = new EventEmitter().sock.remote;\n" +
			"const b: number = connect().remote;\n" +
			"const c: number = path.join('x');\n" +
			"const ok: string = new EventEmitter().on('x').sock.remote;\n",
	}
	for p, content := range files {
		full := filepath.Join(root, filepath.FromSlash(p))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	wd, _ := os.Getwd()
	defer os.Chdir(wd)
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}

	p := NewPaserati()
	p.AddResolver(nodemodules.NewResolver(nodemodules.Options{Mode: nodemodules.Types}))
	var got []string
	for _, r := range p.CheckFilesWithOptions([]string{"main.ts"}, CheckOptions{Types: []string{"node"}, TypesFrom: root}) {
		if r.LoadError != nil {
			t.Fatalf("%s: %v", r.Path, r.LoadError)
		}
		for _, d := range r.Diagnostics {
			got = append(got, fmt.Sprintf("%s %d:%d %s", filepath.Base(r.Path), d.Pos().Line, d.Pos().Column, d.Code()))
		}
	}
	want := []string{"main.ts 4:7 TS2322", "main.ts 5:7 TS2322", "main.ts 6:7 TS2322"}
	if strings.Join(got, ", ") != strings.Join(want, ", ") {
		t.Fatalf("diagnostics = %v, want %v", got, want)
	}
}
