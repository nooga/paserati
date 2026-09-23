package main

import (
	"fmt"
	"strings"
)

// testMeta is the part of a Test262 frontmatter block the runner acts on
// (see test262's INTERPRETING.md).
type testMeta struct {
	Flags    map[string]bool
	Includes []string
	Features []string
	Negative *negativeMeta
}

// negativeMeta is `negative: {phase, type}`: the test passes only if an
// error of Type is raised in Phase (parse, resolution or runtime).
type negativeMeta struct {
	Phase string
	Type  string
}

func (m testMeta) flag(name string) bool { return m.Flags[name] }

// parseMeta reads the /*--- ... ---*/ frontmatter. A file without one has
// empty metadata; a malformed negative block is an error so it can't
// silently turn a negative test into a positive one.
func parseMeta(src string) (testMeta, error) {
	meta := testMeta{Flags: map[string]bool{}}
	hdr := extractFrontmatterHeader(src)
	if hdr == "" {
		return meta, nil
	}
	// Any ECMAScript line terminator ends a frontmatter line (some tests
	// are written with bare CR on purpose).
	lines := strings.FieldsFunc(strings.ReplaceAll(hdr, "\r\n", "\n"), func(r rune) bool {
		return r == '\n' || r == '\r' || r == '\u2028' || r == '\u2029'
	})
	for i := 0; i < len(lines); i++ {
		line := lines[i]
		if line == "" || line[0] == ' ' || line[0] == '\t' {
			continue
		}
		colon := strings.IndexByte(line, ':')
		if colon <= 0 {
			continue
		}
		key := strings.TrimSpace(line[:colon])
		rest := strings.TrimSpace(line[colon+1:])
		// Indented continuation lines belong to this key.
		var block []string
		for i+1 < len(lines) && (lines[i+1] == "" || lines[i+1][0] == ' ' || lines[i+1][0] == '\t') {
			i++
			if t := strings.TrimSpace(lines[i]); t != "" {
				block = append(block, t)
			}
		}
		switch key {
		case "flags", "includes", "features":
			items := yamlList(rest, block)
			switch key {
			case "flags":
				for _, f := range items {
					meta.Flags[f] = true
				}
			case "includes":
				meta.Includes = items
			case "features":
				meta.Features = items
			}
		case "negative":
			neg := &negativeMeta{}
			fields := block
			if strings.HasPrefix(rest, "{") {
				fields = strings.Split(strings.Trim(rest, "{}"), ",")
			}
			for _, f := range fields {
				k, v, ok := strings.Cut(f, ":")
				if !ok {
					continue
				}
				v = unquote(strings.TrimSpace(v))
				switch strings.TrimSpace(k) {
				case "phase":
					neg.Phase = v
				case "type":
					neg.Type = v
				}
			}
			switch neg.Phase {
			case "parse", "resolution", "runtime":
			default:
				return meta, fmt.Errorf("negative metadata has invalid phase %q", neg.Phase)
			}
			if neg.Type == "" {
				return meta, fmt.Errorf("negative metadata has no type")
			}
			meta.Negative = neg
		}
	}
	return meta, nil
}

// yamlList reads a flow list (`[a, b]`, possibly continued on following
// lines) or a block list (`- a` lines).
func yamlList(rest string, block []string) []string {
	var out []string
	if strings.HasPrefix(rest, "[") {
		flow := rest
		for _, b := range block {
			if strings.Contains(flow, "]") {
				break
			}
			flow += " " + b
		}
		flow = strings.TrimPrefix(flow, "[")
		if end := strings.IndexByte(flow, ']'); end >= 0 {
			flow = flow[:end]
		}
		for _, item := range strings.Split(flow, ",") {
			if item = unquote(strings.TrimSpace(item)); item != "" {
				out = append(out, item)
			}
		}
		return out
	}
	for _, b := range block {
		if item, ok := strings.CutPrefix(b, "-"); ok {
			if item = unquote(strings.TrimSpace(item)); item != "" {
				out = append(out, item)
			}
		}
	}
	return out
}

func unquote(s string) string {
	if len(s) >= 2 && (s[0] == '\'' || s[0] == '"') && s[len(s)-1] == s[0] {
		return s[1 : len(s)-1]
	}
	return s
}

// Execution variants (INTERPRETING.md "Strict Mode"): a test with none of
// onlyStrict/noStrict/module/raw must pass both as sloppy code and with a
// "use strict" directive prepended.
const (
	variantSloppy = "non-strict"
	variantStrict = "strict"
	variantModule = "module"
	variantRaw    = "raw"
)

// variantsFor lists the variants a test must pass. strictOnly (the
// -strict-only flag) drops the sloppy variants; a test left with none is
// skipped.
func variantsFor(meta testMeta, strictOnly bool) []string {
	switch {
	case meta.flag("raw"):
		if strictOnly {
			return nil
		}
		return []string{variantRaw}
	case meta.flag("module"):
		return []string{variantModule}
	case meta.flag("onlyStrict"):
		return []string{variantStrict}
	case meta.flag("noStrict"):
		if strictOnly {
			return nil
		}
		return []string{variantSloppy}
	}
	if strictOnly {
		return []string{variantStrict}
	}
	return []string{variantSloppy, variantStrict}
}
