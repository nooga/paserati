package checker

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/nooga/paserati/pkg/modules"
	"github.com/nooga/paserati/pkg/nodemodules"
	"github.com/nooga/paserati/pkg/parser"
)

// SetNodeModuleResolution turns on node16/nodenext import rules: a file's
// format (CommonJS or ECMAScript) follows its extension and package.json
// "type", a CommonJS file cannot import an ECMAScript module (TS1479), and an
// ECMAScript file needs explicit extensions on relative imports (TS2834).
//
// requireESMError is whether require of an ECMAScript module is an error,
// which module node16 and node18 report and node20 and nodenext allow.
func (c *Checker) SetNodeModuleResolution(on, requireESMError bool) {
	c.nodeModuleResolution, c.nodeRequireESMError = on, requireESMError
}

// relativeExtensions are the extensions that count as explicit in an
// ECMAScript module's relative import.
var relativeExtensions = map[string]bool{
	".js": true, ".mjs": true, ".cjs": true, ".jsx": true, ".json": true,
	".ts": true, ".mts": true, ".cts": true, ".tsx": true,
}

// checkNodeImportExtension reports a relative import without a file
// extension in an ECMAScript file, which does not resolve under node16
// rules. It reports whether it did; the report replaces TS2307.
func (c *Checker) checkNodeImportExtension(source *parser.StringLiteral, loaded interface{}) bool {
	spec := source.Value
	if !c.nodeModuleResolution || !(strings.HasPrefix(spec, "./") || strings.HasPrefix(spec, "../")) {
		return false
	}
	from := c.currentFilePath()
	if from == "" || !nodemodules.IsESMFile(from) || relativeExtensions[strings.ToLower(filepath.Ext(spec))] {
		return false
	}
	const msg = "Relative import paths need explicit file extensions in ECMAScript imports when '--moduleResolution' is 'node16' or 'nodenext'."
	rec, _ := loaded.(*modules.ModuleRecord)
	if rec == nil || strings.HasPrefix(rec.ResolvedPath, "ambient:") {
		c.addErrorWithCode(source, "TS2834", msg+" Consider adding an extension to the import path.")
		return true
	}
	c.addErrorWithCode(source, "TS2835", msg+fmt.Sprintf(" Did you mean '%s'?", suggestedExtension(spec, rec.ResolvedPath)))
	return true
}

// suggestedExtension is the specifier an ECMAScript file would write for the
// file spec resolved to: .ts reads as .js, .mts as .mjs, .cts as .cjs, and a
// directory as its index.
func suggestedExtension(spec, resolved string) string {
	base := filepath.Base(resolved)
	js := base
	for _, m := range [][2]string{{".d.mts", ".mjs"}, {".d.cts", ".cjs"}, {".d.ts", ".js"}, {".mts", ".mjs"}, {".cts", ".cjs"}, {".tsx", ".js"}, {".ts", ".js"}} {
		if strings.HasSuffix(base, m[0]) {
			js = strings.TrimSuffix(base, m[0]) + m[1]
			break
		}
	}
	stem := strings.TrimSuffix(js, filepath.Ext(js))
	if stem == "index" && filepath.Base(spec) != "index" {
		return strings.TrimSuffix(spec, "/") + "/" + js
	}
	return spec + filepath.Ext(js)
}

// checkNodeImportFormat reports TS1479 when a CommonJS file imports an
// ECMAScript module, which would be a require call.
func (c *Checker) checkNodeImportFormat(source *parser.StringLiteral, loaded interface{}) {
	if !c.nodeModuleResolution || !c.nodeRequireESMError {
		return
	}
	rec, _ := loaded.(*modules.ModuleRecord)
	from := c.currentFilePath()
	if rec == nil || from == "" || strings.HasPrefix(rec.ResolvedPath, "ambient:") || strings.EqualFold(filepath.Ext(rec.ResolvedPath), ".json") {
		return
	}
	if nodemodules.IsESMFile(from) || !nodemodules.IsESMFile(rec.ResolvedPath) {
		return
	}
	c.addErrorWithCode(source, "TS1479", fmt.Sprintf("The current file is a CommonJS module whose imports will produce 'require' calls; however, the referenced file is an ECMAScript module and cannot be imported with 'require'. Consider writing a dynamic 'import(\"%s\")' call instead.", source.Value))
}
