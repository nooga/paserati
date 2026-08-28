package vm

import (
	"net/url"
	"path/filepath"
	"strings"
)

var opaqueModuleNames = map[string]struct{}{
	"__code_module__": {},
	"__temp_module__": {},
	"<script>":        {},
	"<eval>":          {},
}

// ImportMetaURL is the string stored on import.meta.url for modulePath.
func ImportMetaURL(modulePath string) string {
	return ImportMetaURLWithBase(modulePath, "")
}

// ImportMetaURLWithBase is ImportMetaURL, resolving relative filesystem paths
// against baseDir (the module resolver's root) instead of the process cwd.
func ImportMetaURLWithBase(modulePath, baseDir string) string {
	if modulePath == "" {
		return ""
	}
	if strings.Contains(modulePath, "://") {
		return modulePath
	}
	if _, opaque := opaqueModuleNames[modulePath]; opaque {
		return modulePath
	}
	if !isFilesystemPath(modulePath) {
		return modulePath
	}

	fsPath := modulePath
	if !filepath.IsAbs(fsPath) && baseDir != "" {
		fsPath = filepath.Join(baseDir, fsPath)
	}
	absPath, err := filepath.Abs(fsPath)
	if err != nil {
		absPath = fsPath
	}
	return pathToFileURL(absPath)
}

func isFilesystemPath(modulePath string) bool {
	if filepath.IsAbs(modulePath) {
		return true
	}
	if strings.ContainsAny(modulePath, `/\`) {
		return true
	}
	return filepath.Ext(modulePath) != ""
}

func pathToFileURL(absPath string) string {
	slashPath := filepath.ToSlash(absPath)
	if !strings.HasPrefix(slashPath, "/") {
		slashPath = "/" + slashPath
	}
	u := url.URL{Scheme: "file", Path: slashPath}
	return u.String()
}
