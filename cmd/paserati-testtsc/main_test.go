package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"
)

// TestMain lets the test binary double as a fake worker, so the pool's deadline
// and crash handling can be exercised without the real checker.
func TestMain(m *testing.M) {
	if mode := os.Getenv("TESTTSC_FAKE_WORKER"); mode != "" {
		fakeWorker(mode)
		return
	}
	os.Exit(m.Run())
}

func fakeWorker(mode string) {
	resp := os.NewFile(3, "results")
	dec := json.NewDecoder(os.Stdin)
	enc := json.NewEncoder(resp)
	for {
		var req workerReq
		if dec.Decode(&req) != nil {
			return
		}
		switch {
		case strings.HasPrefix(req.Rel, "hang"):
			time.Sleep(time.Hour)
		case strings.HasPrefix(req.Rel, "crash"):
			os.Stderr.WriteString("fatal error: stack overflow\n\nruntime stack:\n")
			os.Exit(2)
		case strings.HasPrefix(req.Rel, "panic"):
			_ = enc.Encode(workerResp{Panic: "panic: boom"})
		default:
			_ = enc.Encode(workerResp{Actual: []Diag{{Line: 1, Code: "TS1000"}}, CheckRan: true})
		}
	}
}

func TestPoolDeadlineCrashAndDeterminism(t *testing.T) {
	t.Setenv("TESTTSC_FAKE_WORKER", "1")
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	jobs := []workerReq{{Rel: "ok1"}, {Rel: "hang1"}, {Rel: "ok2"}, {Rel: "crash1"}, {Rel: "ok3"}, {Rel: "panic1"}, {Rel: "ok4"}, {Rel: "hang2"}, {Rel: "ok5"}}
	var want []string
	for n, workers := range []int{1, 3} {
		out := runPool(exe, "", jobs, workers, 300*time.Millisecond, nil)
		var got []string
		for i, o := range out {
			got = append(got, jobs[i].Rel+":"+o.Status)
		}
		if n == 0 {
			want = got
			expect := []string{"ok1:ok", "hang1:timeout", "ok2:ok", "crash1:crash", "ok3:ok", "panic1:panic", "ok4:ok", "hang2:timeout", "ok5:ok"}
			if !reflect.DeepEqual(got, expect) {
				t.Fatalf("statuses = %v, want %v", got, expect)
			}
			if !strings.Contains(out[3].Detail, "stack overflow") {
				t.Errorf("crash detail should carry the fatal error line, got %q", out[3].Detail)
			}
		} else if !reflect.DeepEqual(got, want) {
			t.Errorf("-j %d gave %v, -j 1 gave %v", workers, got, want)
		}
	}
}

func TestStripDirectives(t *testing.T) {
	tests := []struct {
		name, in, want string
		opts           map[string]string
	}{
		{
			name: "directives removed, line numbers shift",
			in:   "// @strict: true\n// @target: es5\nlet a = 1;\nlet b = 2;\n",
			want: "let a = 1;\nlet b = 2;\n",
			opts: map[string]string{"strict": "true", "target": "es5"},
		},
		{
			// The harness appends a "\n" separator only when the content so far is
			// non-empty, so blank lines before the first real line vanish.
			name: "leading blank lines vanish",
			in:   "// @strict: true\n\n\nlet a = 1;\n\nlet b;\n",
			want: "let a = 1;\n\nlet b;\n",
			opts: map[string]string{"strict": "true"},
		},
		{
			name: "directive in the middle is removed, blank lines after content stay",
			in:   "let a;\n\n// @noEmit: true\nlet b;",
			want: "let a;\n\nlet b;",
			opts: map[string]string{"noemit": "true"},
		},
		{
			name: "CRLF normalised to LF",
			in:   "// @target: esnext\r\nlet a;\r\nlet b;\r\n",
			want: "let a;\nlet b;\n",
			opts: map[string]string{"target": "esnext"},
		},
		{
			name: "no space, odd spacing and case are still directives",
			in:   "//@Target:ES5\n//   @module  :  commonjs  \nx;",
			want: "x;",
			opts: map[string]string{"target": "ES5", "module": "commonjs"},
		},
		{
			name: "indented directive is code, not a directive",
			in:   "  // @strict: true\nx;",
			want: "  // @strict: true\nx;",
			opts: map[string]string{},
		},
		{
			name: "directive-like text that is not at line start is kept",
			in:   "x; // @strict: true\n",
			want: "x; // @strict: true\n",
			opts: map[string]string{},
		},
		{
			name: "link lines are consumed",
			in:   "// @link: /a -> /b\nx;",
			want: "x;",
			opts: map[string]string{},
		},
		{
			name: "no directives is the identity",
			in:   "a\n\nb",
			want: "a\n\nb",
			opts: map[string]string{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := stripDirectives(tt.in)
			if got.Source != tt.want {
				t.Errorf("source = %q, want %q", got.Source, tt.want)
			}
			if !reflect.DeepEqual(map[string]string(got.Options), tt.opts) {
				t.Errorf("options = %v, want %v", got.Options, tt.opts)
			}
		})
	}
}

func TestIsMultiFileIsCaseInsensitive(t *testing.T) {
	for _, in := range []string{"// @filename: a.ts\nx", "// @Filename: a.ts\nx", "//@fileName:a.ts\nx"} {
		if !isMultiFile(in) {
			t.Errorf("isMultiFile(%q) = false", in)
		}
	}
	// Mentioning the word is not a directive.
	for _, in := range []string{"// see @filename in the docs\nx", "let s = '// @filename: x';"} {
		if isMultiFile(in) {
			t.Errorf("isMultiFile(%q) = true", in)
		}
	}
	single, multi := filterSingleFileTests(func(p string) ([]byte, error) {
		return []byte(map[string]string{"a": "x", "b": "// @Filename: f.ts\ny"}[p]), nil
	}, []string{"a", "b"})
	if !reflect.DeepEqual(single, []string{"a"}) || !reflect.DeepEqual(multi, []string{"b"}) {
		t.Errorf("filterSingleFileTests = %v, %v", single, multi)
	}
}

const chainBaseline = `foo.ts(3,5): error TS2322: Type 'string' is not assignable to type 'number'.
foo.ts(3,5): error TS2322: Type 'string' is not assignable to type 'number'.
foo.ts(7,1): error TS2304: Cannot find name 'x'.
  Types of property 'a' are incompatible.
    Type 'string' is not assignable to type 'number'.
lib.es5.d.ts(--,--): error TS2374: Duplicate index signature for type 'number'.
other.d.ts(2,3): error TS2300: Duplicate identifier 'y'.
error TS2318: Cannot find global type 'Array'.
error TS5107: Option 'target=ES5' is deprecated.


!!! error TS5107: Option 'target=ES5' is deprecated.
==== foo.ts (3 errors) ====
    let a: number = "";
         ~
!!! error TS2322: Type 'string' is not assignable to type 'number'.
    foo.ts(9,9): error TS9999: this is quoted source text, not a diagnostic
`

func TestParseBaseline(t *testing.T) {
	bl := parseBaseline(chainBaseline, "foo.ts")
	wantDiags := []Diag{
		{Line: 3, Col: 5, Code: "TS2322", Msg: "Type 'string' is not assignable to type 'number'."},
		{Line: 3, Col: 5, Code: "TS2322", Msg: "Type 'string' is not assignable to type 'number'."},
		{Line: 7, Col: 1, Code: "TS2304", Msg: "Cannot find name 'x'."},
	}
	if !reflect.DeepEqual(bl.Diags, wantDiags) {
		t.Errorf("Diags = %+v, want %+v", bl.Diags, wantDiags)
	}
	if len(bl.Other) != 2 || bl.Other[0].File != "lib.es5.d.ts" || bl.Other[0].Line != 0 || bl.Other[1].File != "other.d.ts" || bl.Other[1].Line != 2 {
		t.Errorf("Other = %+v", bl.Other)
	}
	if len(bl.Global) != 2 || bl.Global[0].Code != "TS2318" || bl.Global[1].Code != "TS5107" {
		t.Errorf("Global = %+v", bl.Global)
	}
}

func TestParseBaselineOnlyNonSourceDiagnosticsIsClean(t *testing.T) {
	bl := parseBaseline("lib.es5.d.ts(--,--): error TS2411: lib diagnostic\n\n==== zero.ts (0 errors) ====\n    class C {}\n", "zero.ts")
	if len(bl.Diags) != 0 || len(bl.Other) != 1 {
		t.Errorf("got %+v", bl)
	}
}

func TestSplitVaryBy(t *testing.T) {
	tests := []struct {
		key, in string
		want    []string
	}{
		{"strict", "true", nil},
		{"strict", "true,", nil}, // one entry: no variation
		{"strict", "true, false", []string{"true", "false"}},
		{"strict", "TRUE,False,true", []string{"true", "false"}}, // dedupe, lower-case
		{"target", "es5, es2015", []string{"es5", "es2015"}},
		{"target", "es6, es2015", []string{"es6"}}, // alias dedupe by value
		{"target", "esnext", nil},
		{"target", "*,-es3,-es5", nil}, // see below: expands to everything else
		{"lib", "es5,dom", nil},        // list option never varies
		{"strict", "*", []string{"true", "false"}},
		{"strict", "*,-false", []string{"true"}},
		{"types", "*", nil},
	}
	for _, tt := range tests {
		got := splitVaryBy(tt.key, tt.in)
		if tt.key == "target" && tt.in == "*,-es3,-es5" {
			if len(got) < 5 || contains(got, "es3") || contains(got, "es5") || !contains(got, "esnext") {
				t.Errorf("splitVaryBy(%q,%q) = %v", tt.key, tt.in, got)
			}
			continue
		}
		if !reflect.DeepEqual(got, tt.want) {
			t.Errorf("splitVaryBy(%q,%q) = %v, want %v", tt.key, tt.in, got, tt.want)
		}
	}
}

func contains(xs []string, s string) bool {
	for _, x := range xs {
		if x == s {
			return true
		}
	}
	return false
}

func TestExpandVariantsAndSuffix(t *testing.T) {
	vs, tooMany := expandVariants(Options{"strict": "true, false", "target": "es5, es2015", "lib": "es5,dom", "noemit": "true"})
	if tooMany || len(vs) != 4 {
		t.Fatalf("got %d variants (tooMany=%v): %v", len(vs), tooMany, vs)
	}
	var ids []string
	for _, v := range vs {
		ids = append(ids, v.Suffix())
	}
	sort.Strings(ids)
	want := []string{"(strict=false,target=es2015)", "(strict=false,target=es5)", "(strict=true,target=es2015)", "(strict=true,target=es5)"}
	if !reflect.DeepEqual(ids, want) {
		t.Errorf("suffixes = %v, want %v", ids, want)
	}

	vs, _ = expandVariants(Options{"strict": "true", "target": "esnext"})
	if len(vs) != 1 || vs[0].Suffix() != "" {
		t.Errorf("no-variation test should yield one empty variant, got %v", vs)
	}

	_, tooMany = expandVariants(Options{"module": "commonjs,amd,umd,system,es2015", "target": "es5,es2015,es2016,es2017,es2018,es2019"})
	if !tooMany {
		t.Error("30 combinations should exceed TypeScript's cap of 25")
	}
}

func TestBaselineSelectionPerVariant(t *testing.T) {
	dir := t.TempDir()
	write := func(name, content string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("multi(strict=true).errors.txt", "multi.ts(2,1): error TS2322: x\n\n==== multi.ts (1 errors) ====\n")
	// (strict=false) deliberately has no baseline: expected clean.
	write("plain.errors.txt", "plain.ts(1,1): error TS2304: y\n")
	write("two(strict=false,target=es5).errors.txt", "two.ts(1,1): error TS2304: z\n")
	write("alias(target=es2015).errors.txt", "alias.ts(1,1): error TS2304: z\n")

	idx, err := buildBaselineIndex(dir)
	if err != nil {
		t.Fatal(err)
	}
	check := func(name string, v Variant, wantFile string, wantUnmatched bool) {
		t.Helper()
		f, un := idx.lookup(name, v)
		if f != wantFile || un != wantUnmatched {
			t.Errorf("lookup(%s,%v) = %q,%v want %q,%v", name, v, f, un, wantFile, wantUnmatched)
		}
	}
	check("multi", Variant{"strict": "true"}, "multi(strict=true).errors.txt", false)
	check("multi", Variant{"strict": "false"}, "", true) // missing file: expect clean, flagged as unmatched
	check("plain", nil, "plain.errors.txt", false)
	check("nothing", nil, "", false)
	check("two", Variant{"target": "es5", "strict": "false"}, "two(strict=false,target=es5).errors.txt", false)
	check("alias", Variant{"target": "es6"}, "alias(target=es2015).errors.txt", false) // es6 == es2015

	bl, err := idx.load("multi(strict=true).errors.txt", "multi.ts")
	if err != nil || len(bl.Diags) != 1 || bl.Diags[0].Line != 2 {
		t.Errorf("load = %+v, %v", bl, err)
	}
	bl, err = idx.load("", "multi.ts")
	if err != nil || len(bl.Diags) != 0 {
		t.Errorf("missing baseline must load as clean, got %+v, %v", bl, err)
	}
}

func TestOptionsStrictFamilyDefaults(t *testing.T) {
	tests := []struct {
		name string
		opts Options
		want bool
	}{
		{"TS 6.0 default is on", Options{}, true},
		{"strict enables", Options{"strict": "true"}, true},
		{"strict false disables", Options{"strict": "false"}, false},
		{"explicit overrides strict", Options{"strict": "true", "strictpropertyinitialization": "false"}, false},
		{"explicit enables under strict false", Options{"strict": "false", "strictpropertyinitialization": "true"}, true},
		{"trailing comma is a single value", Options{"strict": "false,"}, false},
	}
	for _, tt := range tests {
		if got := tt.opts.strictFamily("strictpropertyinitialization"); got != tt.want {
			t.Errorf("%s: strictFamily = %v, want %v", tt.name, got, tt.want)
		}
	}
	// TS2564 needs strictNullChecks as well.
	if strictPropertyInitEnabled(Options{"strictnullchecks": "false"}) {
		t.Error("strictPropertyInitEnabled must be off without strictNullChecks")
	}
}

func TestUnsupportedOptions(t *testing.T) {
	got := unsupportedOptionsFor(Options{"strict": "false", "target": "ES5", "noimplicitany": "false", "alwaysstrict": "true", "noemit": "true", "filename": ""})
	want := []string{"noimplicitany=false", "strict(partial)", "target=es5"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("unsupportedOptionsFor = %v, want %v", got, want)
	}
	if got := unsupportedOptionsFor(Options{"strict": "true", "noimplicitoverride": "true"}); len(got) != 0 {
		t.Errorf("modelled options must not be reported: %v", got)
	}
}

func d(line, col int, code string) Diag {
	return Diag{Line: line, Col: col, Code: code, Msg: code + " msg"}
}

func TestScoreTiers(t *testing.T) {
	exp := []Diag{d(3, 5, "TS2322"), d(3, 9, "TS2322"), d(7, 1, "TS2304")}
	type verdict struct{ loose, sup, codes, lc, full bool }
	tests := []struct {
		name string
		act  []Diag
		want verdict
	}{
		{"exact", []Diag{d(3, 5, "TS2322"), d(3, 9, "TS2322"), d(7, 1, "TS2304")}, verdict{true, true, true, true, true}},
		{"right line and code, wrong column", []Diag{d(3, 1, "TS2322"), d(3, 2, "TS2322"), d(7, 1, "TS2304")}, verdict{true, true, true, true, false}},
		{"duplicate collapsed: count matters for lines", []Diag{d(3, 5, "TS2322"), d(7, 1, "TS2304")}, verdict{true, true, true, false, false}},
		{"extra diagnostic with a known code", []Diag{d(3, 5, "TS2322"), d(3, 9, "TS2322"), d(7, 1, "TS2304"), d(9, 1, "TS2322")}, verdict{true, true, true, false, false}},
		{"extra diagnostic with a new code", []Diag{d(3, 5, "TS2322"), d(3, 9, "TS2322"), d(7, 1, "TS2304"), d(9, 1, "TS2339")}, verdict{true, true, false, false, false}},
		{"extra diagnostic without TS code", []Diag{d(3, 5, "TS2322"), d(3, 9, "TS2322"), d(7, 1, "TS2304"), d(9, 1, "PS2001")}, verdict{true, true, false, false, false}},
		{"right codes, wrong line", []Diag{d(3, 5, "TS2322"), d(3, 9, "TS2322"), d(8, 1, "TS2304")}, verdict{true, true, true, false, false}},
		{"missing a code", []Diag{d(3, 5, "TS2322"), d(3, 9, "TS2322")}, verdict{true, false, false, false, false}},
		{"no errors at all", nil, verdict{false, false, false, false, false}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := scoreAll(exp, tt.act)
			g := verdict{got[TierLoose].Pass, got[TierCodesSuperset].Pass, got[TierCodesExact].Pass, got[TierLineCodeExact].Pass, got[TierFullExact].Pass}
			if g != tt.want {
				t.Errorf("verdicts = %+v, want %+v", g, tt.want)
			}
		})
	}
}

func TestScoreFullExactComparesFirstMessageLine(t *testing.T) {
	exp := []Diag{{Line: 1, Col: 1, Code: "TS2322", Msg: "Type 'a' is not assignable to type 'b'."}}
	act := []Diag{{Line: 1, Col: 1, Code: "TS2322", Msg: "Type 'a' is not assignable to type 'b'.\n  chain"}}
	if !scoreAll(exp, act)[TierFullExact].Pass {
		t.Error("only the first message line should be compared")
	}
	act[0].Msg = "different"
	r := scoreAll(exp, act)
	if r[TierFullExact].Pass || !r[TierLineCodeExact].Pass {
		t.Errorf("message mismatch should fail only full-exact: %+v", r)
	}
}

func TestScoreExpectedClean(t *testing.T) {
	r := scoreAll(nil, nil)
	for _, tier := range allTiers {
		if !r[tier].Pass || r[tier].Category != "clean-pass" {
			t.Errorf("%s: clean/clean should be a clean-pass, got %+v", tier, r[tier])
		}
	}
	r = scoreAll(nil, []Diag{d(1, 1, "TS2304")})
	for _, tier := range allTiers {
		if r[tier].Pass || r[tier].Category != "clean-fail" {
			t.Errorf("%s: clean/errors should be a clean-fail, got %+v", tier, r[tier])
		}
	}
	// Categories for expected-error tests.
	if got := scoreAll([]Diag{d(1, 1, "TS2304")}, []Diag{d(1, 1, "TS2304")})[TierLineCodeExact].Category; got != "error-match" {
		t.Errorf("category = %s", got)
	}
	if got := scoreAll([]Diag{d(1, 1, "TS2304")}, nil)[TierLoose].Category; got != "error-mismatch" {
		t.Errorf("category = %s", got)
	}
}

func TestScoreDetailNamesTheDifference(t *testing.T) {
	r := scoreAll([]Diag{d(3, 1, "TS2322")}, []Diag{d(4, 1, "TS2322")})[TierLineCodeExact]
	if r.Pass || !strings.Contains(r.Detail, "3:TS2322") || !strings.Contains(r.Detail, "4:TS2322") {
		t.Errorf("detail = %q", r.Detail)
	}
}

func TestDumpHeaderMismatchIsDetectable(t *testing.T) {
	if dumpHeader(TierLoose, "6.0.3") == dumpHeader(TierLineCodeExact, "6.0.3") {
		t.Error("headers for different metrics must differ")
	}
	if got := dumpHeader(TierLineCodeExact, "6.0.3"); got != "# metric=line-code-exact ts=6.0.3" {
		t.Errorf("header = %q", got)
	}
	dir := t.TempDir()
	p := filepath.Join(dir, "dump")
	recs := []Record{
		{ID: "a.ts", Path: "a.ts", Status: StatusOK, Tiers: map[Tier]TierResult{TierLoose: {Pass: true}}},
		{ID: "b.ts(strict=false)", Path: "b.ts", Status: StatusOK, Tiers: map[Tier]TierResult{TierLoose: {Pass: false}}},
		{ID: "c.ts", Path: "c.ts", Status: StatusSkip},
		{ID: "d.ts", Path: "d.ts", Status: StatusTimeout},
	}
	handleDumpMode(recs, p, TierLoose, "6.0.3")
	got, header, err := loadDump(p)
	if err != nil {
		t.Fatal(err)
	}
	if header != "# metric=loose ts=6.0.3" {
		t.Errorf("header = %q", header)
	}
	want := map[string]bool{"a.ts": true, "b.ts(strict=false)": false, "d.ts": false}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("dump = %v, want %v", got, want)
	}
}

// TestRealBaselinesParse cross-checks the baseline parser against the
// `==== file (N errors) ====` headers of every baseline in a TypeScript checkout.
// Set TS_REPO to enable; it is skipped otherwise (unit tests use inline fixtures).
func TestRealBaselinesParse(t *testing.T) {
	repo := os.Getenv("TS_REPO")
	if repo == "" {
		t.Skip("TS_REPO not set")
	}
	dir := filepath.Join(repo, "tests", "baselines", "reference")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Skip(err)
	}
	header := regexp.MustCompile(`(?m)^==== (\S+\.tsx?) \((\d+) errors\) ====`)
	checked, bad := 0, 0
	for _, e := range entries {
		name, _, ok := splitBaselineName(e.Name())
		if !ok {
			continue
		}
		content, _ := os.ReadFile(filepath.Join(dir, e.Name()))
		hs := header.FindAllStringSubmatch(string(content), -1)
		if len(hs) != 1 { // single-file tests only
			continue
		}
		want, _ := strconv.Atoi(hs[0][2])
		bl := parseBaseline(string(content), hs[0][1])
		_ = name
		checked++
		if got := len(bl.Diags) + bl.NonError; got != want {
			bad++
			if bad <= 5 {
				t.Errorf("%s: parsed %d file diagnostics, header says %d", e.Name(), got, want)
			}
		}
	}
	t.Logf("checked %d baselines, %d disagreements", checked, bad)
}
