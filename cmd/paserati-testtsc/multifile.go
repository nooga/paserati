package main

import (
	"os"
	"path"
	"regexp"
	"strings"
)

// This file is the multi-file half of the harness: TypeScript's
// makeUnitsFromTest (the `// @filename:` splitter), the compiler runner's
// root-file selection, and the virtual file system layout its baselines refer
// to. A .tsx test without `@filename` is a multi-file test of one unit: JSX
// parsing depends on the file name.

// testUnit is one `// @filename:` section of a multi-file test.
type testUnit struct {
	Name    string // as written after `@filename:`
	Content string
}

// splitUnits mirrors TestCaseParser.makeUnitsFromTest. Each unit's content has
// its directive lines removed, with the same leading-blank-line quirk as the
// single-file path (baseline line numbers depend on it). A test without
// `@filename` is one unit named after the test file.
func splitUnits(code, testFile string) (units []testUnit, hasLinks bool) {
	var (
		content  string
		haveName bool
		curName  string
	)
	for _, line := range newlineRegex.Split(code, -1) {
		if linkRegex.MatchString(line) {
			hasLinks = true
			continue
		}
		if m := directiveRegex.FindStringSubmatch(line); m != nil {
			if strings.ToLower(m[1]) != "filename" {
				continue
			}
			if haveName {
				units = append(units, testUnit{Name: curName, Content: content})
			}
			curName, haveName = strings.TrimSpace(m[2]), true
			content = ""
			continue
		}
		if content != "" {
			content += "\n"
		}
		content += line
	}
	if !haveName {
		curName = path.Base(strings.ReplaceAll(testFile, "\\", "/"))
	}
	units = append(units, testUnit{Name: curName, Content: content})
	return units, hasLinks
}

// vfsPath is where the harness's virtual file system puts a unit: relative
// names live under /.src, rooted names stay where they are.
func vfsPath(name string) string {
	name = strings.ReplaceAll(name, "\\", "/")
	if strings.HasPrefix(name, "/") {
		return path.Clean(name)
	}
	return path.Clean("/.src/" + name)
}

// displayName is how a baseline prints a file (Utils.removeTestPathPrefixes).
func displayName(vpath string) string {
	return strings.TrimPrefix(vpath, "/.src/")
}

// fileKey normalises a file name for matching units with baseline entries.
func fileKey(name string) string { return strings.ToLower(displayName(vfsPath(name))) }

var (
	requireCallRegex = regexp.MustCompile(`require\(`)
	refPathRegex     = regexp.MustCompile(`reference\spath`)
)

// selectRoots mirrors CompilerTest's choice of root files: when the last unit
// requires or references the others (or noImplicitReferences is set) it is the
// only root and the rest enter by resolution; otherwise every unit is a root.
func selectRoots(units []testUnit, settings Options) []int {
	last := len(units) - 1
	if v, _ := settings.boolOpt("noimplicitreferences"); v ||
		requireCallRegex.MatchString(units[last].Content) || refPathRegex.MatchString(units[last].Content) {
		return []int{last}
	}
	roots := make([]int, 0, len(units))
	for i := range units {
		roots = append(roots, i)
	}
	return roots
}

func baseNameLower(name string) string {
	return strings.ToLower(path.Base(strings.ReplaceAll(name, "\\", "/")))
}

// isCodeUnit reports whether a unit is a source file the compiler takes as a
// root (not package.json and friends).
func isCodeUnit(name string) bool {
	b := baseNameLower(name)
	for _, ext := range []string{".ts", ".tsx", ".mts", ".cts"} {
		if strings.HasSuffix(b, ext) {
			return true
		}
	}
	return false
}

func isJSUnit(name string) bool {
	b := baseNameLower(name)
	for _, ext := range []string{".js", ".jsx", ".mjs", ".cjs"} {
		if strings.HasSuffix(b, ext) {
			return true
		}
	}
	return false
}

// jsChecked reports whether tsc type-checks a JavaScript unit (checkJs, or
// a `// @ts-check` comment) rather than reporting only its syntax errors.
func jsChecked(u testUnit, settings Options) bool {
	if strings.Contains(u.Content, "@ts-nocheck") {
		return false
	}
	if v, _ := settings.boolOpt("checkjs"); v {
		return true
	}
	return strings.Contains(u.Content, "@ts-check")
}

// multiSkipReason says why a multi-file test cannot be run (empty: it can).
// The categories keep the remaining denominator visible.
func multiSkipReason(units []testUnit, settings Options, hasLinks bool) string {
	var tsconfig bool
	for _, u := range units {
		b := baseNameLower(u.Name)
		if b == "tsconfig.json" || b == "jsconfig.json" {
			tsconfig = true
		}
	}
	switch {
	case tsconfig:
		// FIXME: a tsconfig.json unit selects the root files and options.
		return "tsconfig-unit"
	case hasLinks:
		return "symlink"
	}
	return ""
}

// parseBaselineMulti parses a baseline of a multi-file test: diagnostics in any
// unit file are Diags (with File set to the unit's key); everything else (lib
// files, files that are not units) is Other.
func parseBaselineMulti(content string, unitKeys map[string]bool) Baseline {
	all := parseBaseline(content, "\x00no-such-file")
	var bl Baseline
	bl.NonError, bl.Global = all.NonError, all.Global
	for _, d := range all.Other {
		key := strings.ToLower(strings.TrimPrefix(strings.ReplaceAll(d.File, "\\", "/"), "./"))
		if unitKeys[key] && d.Line > 0 {
			d.File = key
			bl.Diags = append(bl.Diags, d)
		} else {
			bl.Other = append(bl.Other, d)
		}
	}
	return bl
}

func unitKeySet(units []testUnit) map[string]bool {
	m := map[string]bool{}
	for _, u := range units {
		m[fileKey(u.Name)] = true
	}
	return m
}

// loadMulti reads the baseline of a multi-file test.
func (bi *baselineIndex) loadMulti(file string, unitKeys map[string]bool) (Baseline, error) {
	if file == "" {
		return Baseline{}, nil
	}
	content, err := os.ReadFile(bi.dir + string(os.PathSeparator) + file)
	if err != nil {
		return Baseline{}, err
	}
	return parseBaselineMulti(string(content), unitKeys), nil
}
