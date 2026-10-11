package tsconfig

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// pattern is a compiled include or exclude pattern.
type pattern struct {
	// root is the longest leading directory without wildcards: a walk for
	// an include pattern starts there.
	root string
	re   *regexp.Regexp
}

// compilePattern compiles a tsc file pattern (an absolute path that may use
// `*`, `?` and `**/`). A pattern naming a directory (no wildcard in its last
// segment and no extension) matches everything under it. For an exclude
// pattern, a match on a directory excludes its whole subtree.
func compilePattern(p string, exclude bool) pattern {
	p = filepath.ToSlash(filepath.Clean(p))
	segs := strings.Split(p, "/")
	last := segs[len(segs)-1]
	if !strings.ContainsAny(last, "*?") {
		if st, err := os.Stat(filepath.FromSlash(p)); err == nil && st.IsDir() {
			p += "/**/*"
			segs = strings.Split(p, "/")
		}
	}
	var root []string
	for _, s := range segs {
		if strings.ContainsAny(s, "*?") {
			break
		}
		root = append(root, s)
	}
	rootDir := strings.Join(root, "/")
	if rootDir == "" {
		rootDir = "/"
	}
	var b strings.Builder
	b.WriteString("^")
	for i, s := range segs {
		if s == "**" {
			// `**/` matches any number of directories, including none.
			if i == len(segs)-1 {
				b.WriteString(".*")
			} else {
				b.WriteString("(?:[^/]+/)*")
			}
			continue
		}
		for _, r := range s {
			switch r {
			case '*':
				b.WriteString("[^/]*")
			case '?':
				b.WriteString("[^/]")
			default:
				b.WriteString(regexp.QuoteMeta(string(r)))
			}
		}
		if i < len(segs)-1 {
			b.WriteString("/")
		}
	}
	if exclude {
		b.WriteString("(?:/.*)?")
	}
	b.WriteString("$")
	return pattern{root: filepath.FromSlash(rootDir), re: regexp.MustCompile(b.String())}
}

func compilePatterns(ps []string, exclude bool) []pattern {
	out := make([]pattern, len(ps))
	for i, p := range ps {
		out[i] = compilePattern(p, exclude)
	}
	return out
}

func (p pattern) match(path string) bool {
	return p.re.MatchString(filepath.ToSlash(path))
}

func matchesAny(ps []pattern, path string) bool {
	for _, p := range ps {
		if p.match(path) {
			return true
		}
	}
	return false
}
