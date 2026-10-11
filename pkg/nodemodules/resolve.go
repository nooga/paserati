// Package nodemodules resolves bare package specifiers ("zod", "@a/b/sub")
// from node_modules directories, the way Node does for running code and the
// way tsc does for type checking.
//
// It is opt-in: the driver never imports it. A host that wants npm packages
// registers a Resolver with Paserati.AddResolver; embedders that don't pay
// nothing for it.
package nodemodules

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Mode selects what a specifier resolves to.
type Mode int

const (
	// Runtime resolves to the file Node would load: exports conditions
	// "node", "import" or "require", "default", then "main", then index.js.
	Runtime Mode = iota
	// Types resolves to the declaration file tsc would read: the "types"
	// condition first, the "types"/"typings" fields, a .d.ts next to the
	// runtime entry, and node_modules/@types/<pkg> as a fallback.
	Types
)

// Options configures a resolution.
type Options struct {
	Mode Mode
	// Require resolves as a require() call: the "require" condition applies
	// instead of "import".
	Require bool
	// Conditions are extra export conditions that apply (Node's
	// --conditions, or tsc's customConditions).
	Conditions []string
	// Paths maps specifiers to files before packages are looked up, as
	// tsc's "paths" option does.
	Paths []PathMapping
}

// PathMapping is one "paths" entry: a pattern with at most one "*" and the
// absolute targets it maps to, the "*" substituted.
type PathMapping struct {
	Pattern string
	Targets []string
}

// ResolvePaths resolves specifier through the "paths" mappings: the pattern
// with the longest prefix matches (an exact pattern before any wildcard), and
// its targets are tried in order. ok is false when no pattern matches.
func ResolvePaths(specifier string, paths []PathMapping, mode Mode) (path string, ok bool, err error) {
	best := -1
	bestPrefix := -1
	star := ""
	for i, m := range paths {
		prefix, suffix, wild := strings.Cut(m.Pattern, "*")
		if !wild {
			if m.Pattern == specifier {
				best, star = i, ""
				break
			}
			continue
		}
		if len(specifier) >= len(prefix)+len(suffix) && strings.HasPrefix(specifier, prefix) &&
			strings.HasSuffix(specifier, suffix) && len(prefix) > bestPrefix {
			best, bestPrefix = i, len(prefix)
			star = specifier[len(prefix) : len(specifier)-len(suffix)]
		}
	}
	if best < 0 {
		return "", false, nil
	}
	for _, target := range paths[best].Targets {
		candidate := strings.Replace(target, "*", star, 1)
		if p, found := tryFile("", candidate, mode); found {
			p, err := canonicalPath(p)
			return p, true, err
		}
	}
	return "", true, fmt.Errorf("cannot find %q through paths mapping %q", specifier, paths[best].Pattern)
}

// IsBareSpecifier reports whether specifier names a package rather than a
// path, URL or builtin.
func IsBareSpecifier(specifier string) bool {
	switch {
	case specifier == "",
		strings.HasPrefix(specifier, "."),
		strings.HasPrefix(specifier, "/"),
		strings.HasPrefix(specifier, "#"),
		strings.HasPrefix(specifier, "node:"),
		strings.Contains(specifier, "://"):
		return false
	}
	return true
}

// Resolve resolves a bare specifier imported from a file in fromDir to an
// absolute path, with symlinks followed.
func Resolve(specifier, fromDir string, opts Options) (string, error) {
	if len(opts.Paths) > 0 {
		if path, ok, err := ResolvePaths(specifier, opts.Paths, opts.Mode); ok && err == nil {
			return path, nil
		}
	}
	pkgName, subpath := SplitSpecifier(specifier)
	if pkgName == "" {
		return "", fmt.Errorf("invalid package specifier %q", specifier)
	}
	dir, err := filepath.Abs(fromDir)
	if err != nil {
		return "", err
	}
	// tsc tries <dir>/node_modules/<pkg> and then <dir>/node_modules/@types/<pkg>
	// in each directory before moving up to the parent.
	for {
		nm := filepath.Join(dir, "node_modules")
		if pkgDir := filepath.Join(nm, filepath.FromSlash(pkgName)); isDir(pkgDir) {
			path, err := resolveInPackage(pkgDir, subpath, opts)
			if err == nil {
				return canonicalPath(path)
			}
			if opts.Mode == Runtime {
				return "", fmt.Errorf("failed to resolve entry for %q: %w", specifier, err)
			}
			// A package without typings: its @types package may have them.
		}
		if opts.Mode == Types && !strings.HasPrefix(pkgName, "@types/") {
			if typesDir := filepath.Join(nm, "@types", filepath.FromSlash(TypesPackageName(pkgName))); isDir(typesDir) {
				if path, err := resolveInPackage(typesDir, subpath, opts); err == nil {
					return canonicalPath(path)
				}
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return "", fmt.Errorf("cannot find package %q from %s", pkgName, fromDir)
}

// ResolveRelative resolves a relative specifier ("./a.js", "../b") imported
// from a file in fromDir the way tsc does: a JavaScript extension stands for
// the TypeScript or declaration file next to it, so "./x.cjs" in a .d.cts
// reads x.d.cts. Only Types mode maps; Runtime mode leaves relative paths to
// the file-system resolver.
func ResolveRelative(specifier, fromDir string, opts Options) (string, error) {
	if opts.Mode != Types {
		return "", fmt.Errorf("relative resolution is only mapped in Types mode")
	}
	base, err := filepath.Abs(filepath.Join(fromDir, filepath.FromSlash(specifier)))
	if err != nil {
		return "", err
	}
	if path, ok := tryFile("", base, Types); ok {
		return canonicalPath(path)
	}
	return "", fmt.Errorf("cannot find %q from %s", specifier, fromDir)
}

// SplitSpecifier splits "pkg/sub/path" or "@scope/pkg/sub/path" into the
// package name and the subpath (without a leading "./").
func SplitSpecifier(specifier string) (pkgName, subpath string) {
	n := 2
	if strings.HasPrefix(specifier, "@") {
		n = 3
	}
	parts := strings.SplitN(specifier, "/", n)
	if len(parts) < n-1 {
		return "", ""
	}
	pkgName = strings.Join(parts[:n-1], "/")
	if len(parts) == n {
		subpath = parts[n-1]
	}
	return pkgName, subpath
}

// TypesPackageName is the name of a package's DefinitelyTyped package under
// @types: "react" -> "react", "@babel/core" -> "babel__core".
func TypesPackageName(pkgName string) string {
	if strings.HasPrefix(pkgName, "@") {
		return strings.Replace(pkgName[1:], "/", "__", 1)
	}
	return pkgName
}

// PackageJSON holds the package.json fields resolution reads.
type PackageJSON struct {
	Name    string
	Main    string
	Types   string // "types", or "typings" when "types" is absent
	Exports json.RawMessage
	Imports json.RawMessage
}

// ReadPackageJSON reads pkgDir/package.json. A field with an unexpected JSON
// type ("main": false is real) reads as empty instead of failing the file.
func ReadPackageJSON(pkgDir string) (*PackageJSON, error) {
	data, err := os.ReadFile(filepath.Join(pkgDir, "package.json"))
	if err != nil {
		return nil, err
	}
	var raw struct {
		Name    json.RawMessage `json:"name"`
		Main    json.RawMessage `json:"main"`
		Types   json.RawMessage `json:"types"`
		Typings json.RawMessage `json:"typings"`
		Exports json.RawMessage `json:"exports"`
		Imports json.RawMessage `json:"imports"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("invalid package.json in %s: %w", pkgDir, err)
	}
	pkg := &PackageJSON{
		Name:    stringField(raw.Name),
		Main:    stringField(raw.Main),
		Types:   stringField(raw.Types),
		Exports: raw.Exports,
		Imports: raw.Imports,
	}
	if pkg.Types == "" {
		pkg.Types = stringField(raw.Typings)
	}
	return pkg, nil
}

func stringField(raw json.RawMessage) string {
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		return ""
	}
	return s
}

func resolveInPackage(pkgDir, subpath string, opts Options) (string, error) {
	pkg, err := ReadPackageJSON(pkgDir)
	if err != nil && !os.IsNotExist(err) {
		return "", err
	}
	if pkg != nil && len(pkg.Exports) > 0 {
		key := "."
		if subpath != "" {
			key = "./" + subpath
		}
		target, ok, err := ExportsTarget(pkg.Exports, key, opts)
		if err != nil {
			return "", err
		}
		if !ok {
			// With an exports map, nothing outside it is reachable.
			return "", fmt.Errorf("%q is not exported by %s", key, pkgDir)
		}
		return entryFile(pkgDir, target, opts.Mode)
	}
	if subpath != "" {
		if path, ok := tryFile(pkgDir, subpath, opts.Mode); ok {
			return path, nil
		}
		return "", fmt.Errorf("subpath %q not found in %s", subpath, pkgDir)
	}
	if pkg != nil {
		if opts.Mode == Types && pkg.Types != "" {
			if path, err := entryFile(pkgDir, pkg.Types, Types); err == nil {
				return path, nil
			}
		}
		// Node ignores the bundler-only "module" field.
		if pkg.Main != "" {
			if path, err := entryFile(pkgDir, pkg.Main, opts.Mode); err == nil {
				return path, nil
			}
		}
	}
	if path, ok := tryFile(pkgDir, "index", opts.Mode); ok {
		return path, nil
	}
	return "", fmt.Errorf("no entry point found in %s", pkgDir)
}

// entryFile maps a package-relative target to a file on disk. In Types mode a
// JavaScript target stands for its declaration file next to it.
func entryFile(pkgDir, target string, mode Mode) (string, error) {
	rel := strings.TrimPrefix(filepath.ToSlash(target), "./")
	if path, ok := tryFile(pkgDir, rel, mode); ok {
		return path, nil
	}
	return "", fmt.Errorf("entry %q does not exist in %s", target, pkgDir)
}

var (
	runtimeExts = []string{"", ".js", ".mjs", ".cjs", ".ts", ".json"}
	runtimeIdx  = []string{"index.js", "index.mjs", "index.ts", "index.json"}
	typesExts   = []string{".ts", ".tsx", ".d.ts"}
	typesIdx    = []string{"index.ts", "index.tsx", "index.d.ts"}
)

// declarationFor maps a JavaScript file name to the declaration file tsc
// pairs with it: x.js -> x.d.ts, x.mjs -> x.d.mts, x.cjs -> x.d.cts.
func declarationFor(rel string) (string, bool) {
	for _, m := range [][2]string{{".mjs", ".d.mts"}, {".cjs", ".d.cts"}, {".jsx", ".d.ts"}, {".js", ".d.ts"}} {
		if base, ok := strings.CutSuffix(rel, m[0]); ok {
			return base + m[1], true
		}
	}
	return "", false
}

func isDeclarationOrTS(rel string) bool {
	for _, ext := range []string{".d.ts", ".d.mts", ".d.cts", ".ts", ".tsx", ".mts", ".cts"} {
		if strings.HasSuffix(rel, ext) {
			return true
		}
	}
	return false
}

func tryFile(pkgDir, rel string, mode Mode) (string, bool) {
	var candidates []string
	if mode == Types {
		if isDeclarationOrTS(rel) {
			candidates = append(candidates, rel)
		} else if dts, ok := declarationFor(rel); ok {
			candidates = append(candidates, dts, strings.TrimSuffix(rel, filepath.Ext(rel))+".ts")
		}
		for _, ext := range typesExts {
			candidates = append(candidates, rel+ext)
		}
		for _, idx := range typesIdx {
			candidates = append(candidates, filepath.ToSlash(filepath.Join(rel, idx)))
		}
	} else {
		for _, ext := range runtimeExts {
			candidates = append(candidates, rel+ext)
		}
		for _, idx := range runtimeIdx {
			candidates = append(candidates, filepath.ToSlash(filepath.Join(rel, idx)))
		}
	}
	for _, c := range candidates {
		path := filepath.Join(pkgDir, filepath.FromSlash(c))
		if isFile(path) {
			return path, true
		}
	}
	return "", false
}

// ExportsTarget looks key ("." or "./sub/path") up in a package.json
// "exports" value and returns the package-relative target for the active
// conditions. Subpath patterns ("./*", "./features/*.js") are matched like
// Node does: the longest matching prefix wins.
func ExportsTarget(exports json.RawMessage, key string, opts Options) (string, bool, error) {
	exports = bytes.TrimSpace(exports)
	if len(exports) == 0 {
		return "", false, nil
	}
	// A string, an array, or an object without "./" keys is the target of "."
	// alone (no other subpaths exist).
	if exports[0] != '{' || !hasSubpathKeys(exports) {
		if key != "." {
			return "", false, nil
		}
		return resolveTarget(exports, "", opts)
	}
	keys, m, ok := orderedObject(exports)
	if !ok {
		return "", false, fmt.Errorf("unsupported exports format")
	}
	if v, exact := m[key]; exact && !strings.Contains(key, "*") {
		return resolveTarget(v, "", opts)
	}
	bestKey, bestStar := "", ""
	for _, k := range keys {
		star := strings.Index(k, "*")
		if star < 0 || strings.Contains(k[star+1:], "*") {
			continue
		}
		prefix, suffix := k[:star], k[star+1:]
		if len(key) < len(prefix)+len(suffix) || !strings.HasPrefix(key, prefix) || !strings.HasSuffix(key, suffix) {
			continue
		}
		if bestKey == "" || patternKeyCompare(k, bestKey) {
			bestKey = k
			bestStar = key[len(prefix) : len(key)-len(suffix)]
		}
	}
	if bestKey == "" {
		return "", false, nil
	}
	return resolveTarget(m[bestKey], bestStar, opts)
}

// patternKeyCompare reports whether pattern a is more specific than b: the
// longer prefix before "*" wins, then the longer key.
func patternKeyCompare(a, b string) bool {
	pa, pb := strings.Index(a, "*"), strings.Index(b, "*")
	if pa != pb {
		return pa > pb
	}
	return len(a) > len(b)
}

func hasSubpathKeys(obj json.RawMessage) bool {
	keys, _, ok := orderedObject(obj)
	if !ok {
		return false
	}
	for _, k := range keys {
		if strings.HasPrefix(k, ".") {
			return true
		}
	}
	return false
}

func (o Options) active(key string) bool {
	switch key {
	case "default":
		return true
	case "node":
		return true
	case "types":
		return o.Mode == Types
	case "require":
		return o.Require
	case "import":
		return !o.Require
	}
	for _, c := range o.Conditions {
		if c == key {
			return true
		}
	}
	return false
}

// resolveTarget resolves an export target: a string (with "*" replaced by
// star), an array of fallbacks, or a conditions object walked in key order.
// A null target excludes the subpath.
func resolveTarget(raw json.RawMessage, star string, opts Options) (string, bool, error) {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || string(raw) == "null" {
		return "", false, nil
	}
	switch raw[0] {
	case '"':
		var s string
		if err := json.Unmarshal(raw, &s); err != nil {
			return "", false, err
		}
		return strings.ReplaceAll(s, "*", star), true, nil
	case '[':
		var alts []json.RawMessage
		if err := json.Unmarshal(raw, &alts); err != nil {
			return "", false, err
		}
		for _, alt := range alts {
			if t, ok, err := resolveTarget(alt, star, opts); err == nil && ok {
				return t, true, nil
			}
		}
		return "", false, nil
	case '{':
		keys, m, ok := orderedObject(raw)
		if !ok {
			return "", false, fmt.Errorf("unsupported export target format")
		}
		for _, k := range keys {
			if !opts.active(k) {
				continue
			}
			t, ok, err := resolveTarget(m[k], star, opts)
			if err != nil {
				return "", false, err
			}
			if ok {
				return t, true, nil
			}
		}
		return "", false, nil
	}
	return "", false, fmt.Errorf("unsupported export target %s", raw)
}

// orderedObject decodes a JSON object keeping its keys in source order, which
// condition matching is defined over.
func orderedObject(raw json.RawMessage) ([]string, map[string]json.RawMessage, bool) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	if tok, err := dec.Token(); err != nil || tok != json.Delim('{') {
		return nil, nil, false
	}
	var keys []string
	m := map[string]json.RawMessage{}
	for dec.More() {
		kt, err := dec.Token()
		if err != nil {
			return nil, nil, false
		}
		k, _ := kt.(string)
		var v json.RawMessage
		if err := dec.Decode(&v); err != nil {
			return nil, nil, false
		}
		if _, dup := m[k]; !dup {
			keys = append(keys, k)
		}
		m[k] = v
	}
	return keys, m, true
}

// canonicalPath makes path absolute with symlinks followed, so one file
// reached through two node_modules paths (pnpm, self-links) is one module.
func canonicalPath(path string) (string, error) {
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		return filepath.Abs(resolved)
	}
	return filepath.Abs(path)
}

func isDir(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

func isFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}
