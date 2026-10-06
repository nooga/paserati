package main

import (
	"os"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// Diag is one diagnostic, expected (from a baseline) or actual (from us).
type Diag struct {
	Line  int    `json:"line"`
	Col   int    `json:"col"`
	Code  string `json:"code"` // TSxxxx; for our own diagnostics without a TS mapping, the PSxxxx code
	Msg   string `json:"msg"`
	Phase string `json:"phase,omitempty"` // actual only: "parse" or "check"
	File  string `json:"file,omitempty"`  // expected only, for other-file diagnostics
}

// isTS reports whether the diagnostic carries a TypeScript code.
func (d Diag) isTS() bool {
	return len(d.Code) > 2 && d.Code[0] == 'T' && d.Code[1] == 'S'
}

// Baseline is a parsed `*.errors.txt` reference baseline.
//
// Treatment of the three kinds of diagnostic a baseline can list (all of them
// are in the summary block at the top of the file; the indented continuation
// lines and the `!!! error` lines in the annotated body below the `====`
// header repeat them and are ignored):
//
//   - Diags: `test.ts(l,c): error TSxxxx: msg` in the test file itself. These
//     are the only diagnostics we score. Lines and columns are 1-based
//     (columns are UTF-16 code units, like ours for the BMP).
//   - Other: diagnostics located in a different file (`lib.es5.d.ts(--,--)`,
//     a referenced `.d.ts`, ...). We only ever check the test file and never
//     emit such diagnostics, so they are recorded but excluded from scoring.
//   - Only `error` diagnostics are scored; `suggestion TS6807` / `message TS1450`
//     lines (which tsc does not print) are counted in NonError and ignored.
//   - Global: `error TSxxxx: msg` with no file (compiler-option diagnostics like
//     TS5107, or TS2318 for missing global types). Also excluded from scoring
//     and recorded. A test whose baseline has no Diags is therefore "expected
//     clean" even if it has Other/Global diagnostics: we cannot produce them,
//     and counting them would make those tests unwinnable for a checker that
//     is otherwise correct.
type Baseline struct {
	Diags    []Diag
	Other    []Diag
	Global   []Diag
	NonError int // `suggestion`/`message`/`warning` diagnostics: counted in the file header, never scored
}

var (
	baselineDiagRegex = regexp.MustCompile(`^(.+?)\((\d+|--),(\d+|--)\): (error|warning|suggestion|message) (TS\d+): (.*)$`)
	// prettyDiagRegex matches the `--pretty` rendering (after ANSI codes are removed).
	prettyDiagRegex     = regexp.MustCompile(`^(.+?):(\d+):(\d+) - (error|warning|suggestion|message) (TS\d+): (.*)$`)
	ansiRegex           = regexp.MustCompile(`\x1b\[[0-9;]*m`)
	baselineGlobalRegex = regexp.MustCompile(`^error (TS\d+): (.*)$`)
)

// parseBaseline parses the summary block of an `.errors.txt` baseline.
// testFile is the test's own file name (e.g. "foo.ts"); diagnostics whose file
// base name equals it (case-insensitively) are in-file diagnostics.
func parseBaseline(content, testFile string) Baseline {
	var bl Baseline
	testBase := strings.ToLower(path.Base(testFile))
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimRight(line, "\r")
		if strings.HasPrefix(line, "====") {
			break // the annotated source body follows; everything in it is a repeat
		}
		if line == "" || line[0] == ' ' || line[0] == '\t' {
			continue // blank or message-chain continuation
		}
		line = ansiRegex.ReplaceAllString(line, "")
		m := baselineDiagRegex.FindStringSubmatch(line)
		if m == nil {
			m = prettyDiagRegex.FindStringSubmatch(line)
		}
		if m != nil {
			if m[4] != "error" {
				bl.NonError++ // suggestions/messages: tsc does not print them, so we never score them
				continue
			}
			d := Diag{Code: m[5], Msg: m[6], File: m[1]}
			if m[2] != "--" {
				d.Line, _ = strconv.Atoi(m[2])
			}
			if m[3] != "--" {
				d.Col, _ = strconv.Atoi(m[3])
			}
			if strings.ToLower(path.Base(m[1])) == testBase && m[2] != "--" {
				d.File = ""
				bl.Diags = append(bl.Diags, d)
			} else {
				bl.Other = append(bl.Other, d)
			}
			continue
		}
		if m := baselineGlobalRegex.FindStringSubmatch(line); m != nil {
			bl.Global = append(bl.Global, Diag{Code: m[1], Msg: m[2]})
		}
	}
	return bl
}

// baselineIndex maps a test's base name to the baseline files that exist for it.
type baselineIndex struct {
	dir    string
	byName map[string]map[string]string // name -> matchKey ("" for plain) -> file name
}

const errorsSuffix = ".errors.txt"

// splitBaselineName splits `name(k=v,k=v).errors.txt` into the test name and the
// variant it encodes. ok is false for files that are not error baselines.
func splitBaselineName(file string) (name string, v Variant, ok bool) {
	if !strings.HasSuffix(file, errorsSuffix) {
		return "", nil, false
	}
	stem := strings.TrimSuffix(file, errorsSuffix)
	if strings.HasSuffix(stem, ")") {
		if i := strings.LastIndex(stem, "("); i > 0 {
			params := stem[i+1 : len(stem)-1]
			v := Variant{}
			valid := true
			for _, kv := range strings.Split(params, ",") {
				eq := strings.Index(kv, "=")
				if eq <= 0 {
					valid = false
					break
				}
				v[strings.ToLower(kv[:eq])] = strings.ToLower(kv[eq+1:])
			}
			if valid {
				return stem[:i], v, true
			}
		}
	}
	return stem, Variant{}, true
}

func buildBaselineIndex(dir string) (*baselineIndex, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	bi := &baselineIndex{dir: dir, byName: make(map[string]map[string]string)}
	for _, e := range entries {
		name, v, ok := splitBaselineName(e.Name())
		if !ok {
			continue
		}
		m := bi.byName[name]
		if m == nil {
			m = make(map[string]string)
			bi.byName[name] = m
		}
		m[v.matchKey()] = e.Name()
	}
	return bi, nil
}

// lookup finds the baseline for a test variant. A missing baseline means the
// harness produced no errors.txt, i.e. the variant is expected clean
// (file == ""). unmatched is true when baselines exist for the test name but none
// belongs to this variant (usually: the variant is clean; but it can also mean
// our variant expansion disagrees with TypeScript's, so it is surfaced).
func (bi *baselineIndex) lookup(name string, v Variant) (file string, unmatched bool) {
	m := bi.byName[name]
	if m == nil {
		return "", false
	}
	if f, ok := m[v.matchKey()]; ok {
		return f, false
	}
	return "", true
}

// variantKeysIn lists the variant keys used by the baselines of a test name
// (for the unmatched audit).
func (bi *baselineIndex) variantKeysIn(name string) []string {
	var keys []string
	for k := range bi.byName[name] {
		if k != "" {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	return keys
}

// loadBaseline reads and parses the baseline for (name, variant). file=="" gives
// an empty (clean) baseline.
func (bi *baselineIndex) load(file, testFile string) (Baseline, error) {
	if file == "" {
		return Baseline{}, nil
	}
	content, err := os.ReadFile(bi.dir + string(os.PathSeparator) + file)
	if err != nil {
		return Baseline{}, err
	}
	return parseBaseline(string(content), testFile), nil
}
