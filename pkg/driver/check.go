package driver

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/nooga/paserati/pkg/errors"
)

// FileDiagnostics is the type-checking result of one file.
type FileDiagnostics struct {
	Path        string
	Diagnostics []errors.PaseratiError
	// LoadError is set when the file could not be loaded or parsed.
	LoadError error
}

// CheckFiles type-checks files and every module they import without
// compiling or running anything (`paserati --check`, tsc --noEmit). It
// returns the results of the files checked, sorted by path, leaving out
// library files as tsc's skipLibCheck does: declaration files and modules
// under node_modules. Bare package imports resolve only if the host has
// registered a resolver for them (pkg/nodemodules in Types mode).
func (p *Paserati) CheckFiles(files []string) []FileDiagnostics {
	p.moduleLoader.SetCompilerFactory(nil)
	// A type error in one module must not hide the types it exports from
	// the modules importing it: every module's diagnostics are kept on its
	// record and reported below.
	p.moduleLoader.SetIgnoreTypeErrors(true)

	wd, err := os.Getwd()
	if err != nil {
		wd = "."
	}
	from := filepath.Join(wd, "<check>")
	var loadErrors []FileDiagnostics
	for _, file := range files {
		abs, err := filepath.Abs(file)
		if err != nil {
			loadErrors = append(loadErrors, FileDiagnostics{Path: file, LoadError: err})
			continue
		}
		rel, err := filepath.Rel(wd, abs)
		if err != nil || strings.HasPrefix(rel, "..") {
			rel = abs
		} else {
			rel = "./" + filepath.ToSlash(rel)
		}
		if _, err := p.moduleLoader.LoadModule(rel, from); err != nil {
			loadErrors = append(loadErrors, FileDiagnostics{Path: abs, LoadError: err})
		}
	}

	results := loadErrors
	for _, rec := range p.moduleLoader.Modules() {
		if rec.ResolvedPath == "" {
			continue
		}
		if rec.Error != nil {
			// A file that does not parse is reported, library or not (tsc
			// reports syntax errors under skipLibCheck too).
			res := FileDiagnostics{Path: rec.ResolvedPath, Diagnostics: rec.Diagnostics}
			if len(rec.Diagnostics) == 0 {
				res.LoadError = rec.Error
			}
			results = append(results, res)
			continue
		}
		if isLibraryFile(rec.ResolvedPath) {
			continue
		}
		results = append(results, FileDiagnostics{Path: rec.ResolvedPath, Diagnostics: rec.Diagnostics})
	}
	sort.SliceStable(results, func(i, j int) bool { return results[i].Path < results[j].Path })
	return results
}

// isLibraryFile reports whether path is a declaration file or lives under
// node_modules, the files tsc's skipLibCheck leaves unchecked.
func isLibraryFile(path string) bool {
	slashed := filepath.ToSlash(path)
	if strings.Contains(slashed, "/node_modules/") {
		return true
	}
	for _, ext := range []string{".d.ts", ".d.mts", ".d.cts"} {
		if strings.HasSuffix(slashed, ext) {
			return true
		}
	}
	return false
}
