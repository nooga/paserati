package modules

import (
	"regexp"
	"strings"
)

var tripleSlashRef = regexp.MustCompile(`^///\s*<reference\s+(path|types)\s*=\s*["']([^"']+)["']`)

// TripleSlashReferences returns the specifiers of a file's leading
// `/// <reference path="..." />` and `/// <reference types="..." />`
// directives: a path as a relative specifier, a types reference as a
// package name. Only the comment lines before the first statement count, as
// in tsc; `lib` references are left to the host's own lib.
func TripleSlashReferences(content string) []string {
	var refs []string
	for len(content) > 0 {
		line := content
		if i := strings.IndexByte(content, '\n'); i >= 0 {
			line, content = content[:i], content[i+1:]
		} else {
			content = ""
		}
		line = strings.TrimSpace(line)
		switch {
		case line == "" || strings.HasPrefix(line, "#!"):
			continue
		case strings.HasPrefix(line, "///"):
			m := tripleSlashRef.FindStringSubmatch(line)
			if m == nil {
				continue
			}
			spec := m[2]
			if m[1] == "path" && !strings.HasPrefix(spec, ".") && !strings.HasPrefix(spec, "/") {
				spec = "./" + spec
			}
			refs = append(refs, spec)
		case strings.HasPrefix(line, "//"):
			continue
		case strings.HasPrefix(line, "/*"):
			// Skip a block comment.
			rest := line + "\n" + content
			end := strings.Index(rest, "*/")
			if end < 0 {
				return refs
			}
			content = rest[end+2:]
		default:
			return refs
		}
	}
	return refs
}
