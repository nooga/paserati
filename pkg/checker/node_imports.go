package checker

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/nooga/paserati/pkg/errors"
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
// extension in an ECMAScript file (or a dynamic import, which always uses the
// ECMAScript resolver), which does not resolve under node16 rules. It
// reports whether it did; the report replaces TS2307.
func (c *Checker) checkNodeImportExtension(source *parser.StringLiteral, dynamic bool) bool {
	spec := source.Value
	if !c.nodeModuleResolution || !(strings.HasPrefix(spec, "./") || strings.HasPrefix(spec, "../")) {
		return false
	}
	from := c.currentFilePath()
	if from == "" || !(dynamic || nodemodules.IsESMFile(from)) || relativeExtensions[strings.ToLower(filepath.Ext(spec))] {
		return false
	}
	if strings.HasSuffix(spec, "/") {
		c.addErrorWithCode(source, errors.TS2307, fmt.Sprintf("Cannot find module '%s' or its corresponding type declarations.", spec))
		return true
	}
	const msg = "Relative import paths need explicit file extensions in ECMAScript imports when '--moduleResolution' is 'node16' or 'nodenext'."
	if suggest, ok := suggestedExtension(from, spec); ok {
		c.addErrorWithCode(source, "TS2835", msg+fmt.Sprintf(" Did you mean '%s'?", suggest))
	} else {
		c.addErrorWithCode(source, "TS2834", msg+" Consider adding an extension to the import path.")
	}
	return true
}

// suggestedExtension is the specifier an ECMAScript file would write for the
// file that spec names once the importing file's extension rules add an
// extension: .ts reads as .js, .mts as .mjs, .cts as .cjs. A directory index
// is no suggestion.
func suggestedExtension(from, spec string) (string, bool) {
	var exts [][2]string
	switch strings.ToLower(filepath.Ext(from)) {
	case ".mts":
		exts = [][2]string{{".mts", ".mjs"}, {".d.mts", ".mjs"}}
	case ".cts":
		exts = [][2]string{{".cts", ".cjs"}, {".d.cts", ".cjs"}}
	default:
		exts = [][2]string{{".ts", ".js"}, {".tsx", ".js"}, {".d.ts", ".js"}}
	}
	base := filepath.Join(filepath.Dir(from), filepath.FromSlash(spec))
	for _, e := range exts {
		if info, err := os.Stat(base + e[0]); err == nil && !info.IsDir() {
			return spec + e[1], true
		}
	}
	return "", false
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

// checkDynamicImportSpecifier resolves the specifier of `import("x")` and
// reports what does not resolve, as an ECMAScript import: TS2307, or TS2834
// and TS2835 for a relative path without extension.
func (c *Checker) checkDynamicImportSpecifier(source *parser.StringLiteral) {
	if c.moduleLoader == nil || c.isSelfModule(source.Value) {
		return
	}
	if c.checkNodeImportExtension(source, true) {
		return
	}
	if _, err := c.moduleLoader.LoadModule(source.Value, c.currentFilePath()); err != nil {
		c.addErrorWithCode(source, errors.TS2307, fmt.Sprintf("Cannot find module '%s' or its corresponding type declarations.", source.Value))
	}
}
