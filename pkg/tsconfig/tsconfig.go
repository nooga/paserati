// Package tsconfig reads a tsconfig.json the way tsc does for a project
// build (`tsc -p`): the JSON-with-comments syntax, "extends" chains, the root
// files from "files"/"include"/"exclude", and the compiler options Paserati's
// checker understands. It is used by the paserati CLI; embedders that do not
// read tsconfig files never import it.
package tsconfig

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/nooga/paserati/pkg/nodemodules"
)

// Config is a resolved project configuration.
type Config struct {
	// Path is the absolute path of the tsconfig.json.
	Path string
	// Files are the project's root files, absolute and sorted.
	Files []string
	// Options are the merged compilerOptions, keys lower-cased.
	Options map[string]any
	// Paths are the "paths" mappings with absolute targets.
	Paths []nodemodules.PathMapping
}

// raw is one config file's content, with its relative paths still relative
// to dir.
type raw struct {
	dir             string
	files           []string
	include         []string
	exclude         []string
	hasFiles        bool
	hasInclude      bool
	hasExclude      bool
	options         map[string]any
	pathsBase       string // directory "paths" targets are relative to
	paths           map[string][]string
	outDir, rootDir string
}

// Load reads the config at path (a file, or a directory holding
// tsconfig.json) with its "extends" chain and finds its root files.
func Load(path string) (*Config, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	if st, err := os.Stat(abs); err == nil && st.IsDir() {
		abs = filepath.Join(abs, "tsconfig.json")
	}
	r, err := load(abs, map[string]bool{})
	if err != nil {
		return nil, err
	}
	cfg := &Config{Path: abs, Options: r.options}
	if r.paths != nil {
		keys := make([]string, 0, len(r.paths))
		for k := range r.paths {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			m := nodemodules.PathMapping{Pattern: k}
			for _, t := range r.paths[k] {
				m.Targets = append(m.Targets, filepath.Join(r.pathsBase, filepath.FromSlash(t)))
			}
			cfg.Paths = append(cfg.Paths, m)
		}
	}
	cfg.Files, err = rootFiles(r, cfg.Options, filepath.Dir(abs))
	if err != nil {
		return nil, err
	}
	return cfg, nil
}

// load reads one config and merges the configs it extends under it.
func load(path string, seen map[string]bool) (*raw, error) {
	if seen[path] {
		return nil, fmt.Errorf("%s: circular extends", path)
	}
	seen[path] = true
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var doc map[string]json.RawMessage
	if err := json.Unmarshal(StripJSONC(data), &doc); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	dir := filepath.Dir(path)

	// The configs this one extends, applied in order, then this one.
	var base *raw
	if ext, ok := doc["extends"]; ok {
		var list []string
		var one string
		if json.Unmarshal(ext, &one) == nil {
			list = []string{one}
		} else if err := json.Unmarshal(ext, &list); err != nil {
			return nil, fmt.Errorf("%s: extends must be a string or an array of strings", path)
		}
		for _, spec := range list {
			extPath, err := resolveExtends(spec, dir)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", path, err)
			}
			parent, err := load(extPath, seen)
			if err != nil {
				return nil, err
			}
			base = merge(base, parent)
		}
	}

	r := &raw{dir: dir, options: map[string]any{}}
	if v, ok := doc["files"]; ok {
		r.hasFiles = true
		if err := json.Unmarshal(v, &r.files); err != nil {
			return nil, fmt.Errorf("%s: files: %w", path, err)
		}
	}
	if v, ok := doc["include"]; ok {
		r.hasInclude = true
		if err := json.Unmarshal(v, &r.include); err != nil {
			return nil, fmt.Errorf("%s: include: %w", path, err)
		}
	}
	if v, ok := doc["exclude"]; ok {
		r.hasExclude = true
		if err := json.Unmarshal(v, &r.exclude); err != nil {
			return nil, fmt.Errorf("%s: exclude: %w", path, err)
		}
	}
	if v, ok := doc["compilerOptions"]; ok {
		var opts map[string]json.RawMessage
		if err := json.Unmarshal(v, &opts); err != nil {
			return nil, fmt.Errorf("%s: compilerOptions: %w", path, err)
		}
		for k, val := range opts {
			key := strings.ToLower(k)
			switch key {
			case "paths":
				if err := json.Unmarshal(val, &r.paths); err != nil {
					return nil, fmt.Errorf("%s: paths: %w", path, err)
				}
				r.pathsBase = dir
			case "baseurl":
				// Paths are relative to baseUrl when it is set (TypeScript
				// before 7), else to the config declaring them.
				var b string
				if json.Unmarshal(val, &b) == nil {
					r.options[key] = filepath.Join(dir, filepath.FromSlash(b))
				}
			case "outdir", "rootdir":
				var d string
				if json.Unmarshal(val, &d) == nil {
					d = filepath.Join(dir, filepath.FromSlash(d))
					r.options[key] = d
					if key == "outdir" {
						r.outDir = d
					} else {
						r.rootDir = d
					}
				}
			default:
				var x any
				if err := json.Unmarshal(val, &x); err == nil {
					r.options[key] = x
				}
			}
		}
	}
	merged := merge(base, r)
	if b, ok := merged.options["baseurl"].(string); ok && merged.paths != nil {
		merged.pathsBase = b
	}
	return merged, nil
}

// merge applies child over base: options are merged key by key; files,
// include and exclude are replaced when the child sets them (and stay
// relative to the config that set them).
func merge(base, child *raw) *raw {
	if base == nil {
		return child
	}
	out := *base
	out.options = map[string]any{}
	for k, v := range base.options {
		out.options[k] = v
	}
	for k, v := range child.options {
		out.options[k] = v
	}
	if child.paths != nil {
		out.paths, out.pathsBase = child.paths, child.pathsBase
	}
	if child.hasFiles || child.hasInclude || child.hasExclude {
		// The list's paths are relative to the config that has it.
		if child.hasFiles {
			out.files, out.hasFiles = absAll(child.dir, child.files), true
		} else {
			out.files = absAll(base.dir, base.files)
		}
		if child.hasInclude {
			out.include, out.hasInclude = absAll(child.dir, child.include), true
		} else {
			out.include = absAll(base.dir, base.include)
		}
		if child.hasExclude {
			out.exclude, out.hasExclude = absAll(child.dir, child.exclude), true
		} else {
			out.exclude = absAll(base.dir, base.exclude)
		}
		out.dir = "" // lists are absolute now
	} else {
		out.files = absAll(base.dir, base.files)
		out.include = absAll(base.dir, base.include)
		out.exclude = absAll(base.dir, base.exclude)
		out.dir = ""
	}
	if child.outDir != "" {
		out.outDir = child.outDir
	}
	if child.rootDir != "" {
		out.rootDir = child.rootDir
	}
	return &out
}

func absAll(dir string, paths []string) []string {
	if paths == nil {
		return nil
	}
	out := make([]string, len(paths))
	for i, p := range paths {
		if dir == "" || filepath.IsAbs(p) {
			out[i] = p
		} else {
			out[i] = filepath.Join(dir, filepath.FromSlash(p))
		}
	}
	return out
}

// resolveExtends finds the config an "extends" names: a path relative to the
// extending config, or a package (`@tsconfig/node18/tsconfig.json`, or the
// package's own tsconfig.json).
func resolveExtends(spec, dir string) (string, error) {
	if strings.HasPrefix(spec, ".") || filepath.IsAbs(spec) {
		p := spec
		if !filepath.IsAbs(p) {
			p = filepath.Join(dir, filepath.FromSlash(spec))
		}
		for _, c := range []string{p, p + ".json"} {
			if st, err := os.Stat(c); err == nil && !st.IsDir() {
				return c, nil
			}
		}
		return "", fmt.Errorf("cannot find extended config %q", spec)
	}
	for d := dir; ; d = filepath.Dir(d) {
		p := filepath.Join(d, "node_modules", filepath.FromSlash(spec))
		for _, c := range []string{p, p + ".json", filepath.Join(p, "tsconfig.json")} {
			if st, err := os.Stat(c); err == nil && !st.IsDir() {
				return c, nil
			}
		}
		if parent := filepath.Dir(d); parent == d {
			break
		}
	}
	return "", fmt.Errorf("cannot find extended config %q", spec)
}

// rootFiles lists the project's root files: "files", plus what "include"
// matches (everything under the config's directory when neither is set),
// minus what "exclude" matches.
func rootFiles(r *raw, options map[string]any, topDir string) ([]string, error) {
	seen := map[string]bool{}
	var out []string
	add := func(p string) {
		if !seen[p] {
			seen[p] = true
			out = append(out, p)
		}
	}
	for _, f := range absAll(r.dir, r.files) {
		if _, err := os.Stat(f); err != nil {
			return nil, fmt.Errorf("file %q not found", f)
		}
		add(f)
	}
	include := absAll(r.dir, r.include)
	if !r.hasInclude && !r.hasFiles {
		include = []string{filepath.Join(topDir, "**", "*")}
	}
	exclude := absAll(r.dir, r.exclude)
	if !r.hasExclude {
		base := topDir
		exclude = []string{
			filepath.Join(base, "node_modules"),
			filepath.Join(base, "bower_components"),
			filepath.Join(base, "jspm_packages"),
		}
		if r.outDir != "" {
			exclude = append(exclude, r.outDir)
		}
	}
	allowJS, _ := options["allowjs"].(bool)
	excluded := compilePatterns(exclude, true)
	for _, inc := range include {
		m := compilePattern(inc, false)
		err := filepath.WalkDir(m.root, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if d.IsDir() {
				name := d.Name()
				if path != m.root && (name == "node_modules" && !strings.Contains(inc, "node_modules") || strings.HasPrefix(name, ".")) {
					return filepath.SkipDir
				}
				if matchesAny(excluded, path) {
					return filepath.SkipDir
				}
				return nil
			}
			if !supported(path, allowJS) || !m.match(path) || matchesAny(excluded, path) {
				return nil
			}
			add(path)
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	sort.Strings(out)
	return out, nil
}

func supported(path string, allowJS bool) bool {
	for _, ext := range []string{".ts", ".tsx", ".mts", ".cts"} {
		if strings.HasSuffix(path, ext) {
			return true
		}
	}
	if allowJS {
		for _, ext := range []string{".js", ".jsx", ".mjs", ".cjs"} {
			if strings.HasSuffix(path, ext) {
				return true
			}
		}
	}
	return false
}

// StripJSONC turns tsconfig's JSON-with-comments into JSON: comments are
// removed and trailing commas dropped. String contents are kept as they are.
func StripJSONC(data []byte) []byte {
	out := make([]byte, 0, len(data))
	inString := false
	for i := 0; i < len(data); i++ {
		ch := data[i]
		if inString {
			out = append(out, ch)
			if ch == '\\' && i+1 < len(data) {
				i++
				out = append(out, data[i])
			} else if ch == '"' {
				inString = false
			}
			continue
		}
		switch {
		case ch == '"':
			inString = true
			out = append(out, ch)
		case ch == '/' && i+1 < len(data) && data[i+1] == '/':
			for i < len(data) && data[i] != '\n' {
				i++
			}
			if i < len(data) {
				out = append(out, '\n')
			}
		case ch == '/' && i+1 < len(data) && data[i+1] == '*':
			i += 2
			for i+1 < len(data) && !(data[i] == '*' && data[i+1] == '/') {
				i++
			}
			i++
		case ch == ',':
			// Drop a comma followed (after whitespace and comments) by a
			// closing bracket.
			j := i + 1
			for j < len(data) {
				if data[j] == ' ' || data[j] == '\t' || data[j] == '\n' || data[j] == '\r' {
					j++
				} else if data[j] == '/' && j+1 < len(data) && data[j+1] == '/' {
					for j < len(data) && data[j] != '\n' {
						j++
					}
				} else if data[j] == '/' && j+1 < len(data) && data[j+1] == '*' {
					j += 2
					for j+1 < len(data) && !(data[j] == '*' && data[j+1] == '/') {
						j++
					}
					j += 2
				} else {
					break
				}
			}
			if j < len(data) && (data[j] == '}' || data[j] == ']') {
				continue
			}
			out = append(out, ch)
		default:
			out = append(out, ch)
		}
	}
	return out
}

// Bool returns a boolean compiler option and whether it is set.
func (c *Config) Bool(name string) (value, set bool) {
	v, ok := c.Options[strings.ToLower(name)].(bool)
	return v, ok
}

// StrictFamily returns a strict-family option: its own setting, else
// "strict", else on (TypeScript 6's default).
func (c *Config) StrictFamily(name string) bool {
	if v, ok := c.Bool(name); ok {
		return v
	}
	if v, ok := c.Bool("strict"); ok {
		return v
	}
	return true
}

// Types returns the type packages whose globals the project includes: the
// "types" option when set, else every package under node_modules/@types in
// the config's directory and its ancestors (tsc's default type roots).
func (c *Config) Types() []string {
	if list, ok := c.Options["types"].([]any); ok {
		out := make([]string, 0, len(list))
		for _, v := range list {
			if s, ok := v.(string); ok {
				out = append(out, s)
			}
		}
		return out
	}
	seen := map[string]bool{}
	var out []string
	for d := filepath.Dir(c.Path); ; d = filepath.Dir(d) {
		entries, _ := os.ReadDir(filepath.Join(d, "node_modules", "@types"))
		for _, e := range entries {
			name := e.Name()
			if strings.HasPrefix(name, ".") || seen[name] {
				continue
			}
			seen[name] = true
			// @types/a__b is the typings of @a/b.
			if scope, pkg, ok := strings.Cut(name, "__"); ok {
				name = "@" + scope + "/" + pkg
			}
			out = append(out, name)
		}
		if parent := filepath.Dir(d); parent == d {
			break
		}
	}
	sort.Strings(out)
	return out
}
