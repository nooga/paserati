package main

import (
	"fmt"
	"sort"
	"strings"
)

// Tier is one definition of "this test passes". Tiers are strictly ordered by
// how much of TypeScript's output they demand.
type Tier string

const (
	TierLoose         Tier = "loose"           // expected errors <=> we reported errors
	TierCodesSuperset Tier = "codes-superset"  // every expected TS code reported (extras allowed)
	TierCodesExact    Tier = "codes-exact"     // set of TS codes equal; non-TS diagnostics count as extra
	TierLineCodeExact Tier = "line-code-exact" // multiset of (line, code) equal  -- the primary metric
	TierFullExact     Tier = "full-exact"      // multiset of (line, col, code, first message line) equal
)

var allTiers = []Tier{TierLoose, TierCodesSuperset, TierCodesExact, TierLineCodeExact, TierFullExact}

func parseTier(s string) (Tier, bool) {
	for _, t := range allTiers {
		if string(t) == s {
			return t, true
		}
	}
	return "", false
}

// TierResult is the verdict of one tier for one test.
type TierResult struct {
	Pass     bool   `json:"pass"`
	Category string `json:"category"` // clean-pass | clean-fail | error-match | error-mismatch
	Detail   string `json:"detail,omitempty"`
}

// scoreAll scores actual against expected (in-file diagnostics only) under every
// tier. Expected-clean means exp is empty.
func scoreAll(exp, act []Diag) map[Tier]TierResult {
	out := make(map[Tier]TierResult, len(allTiers))
	expectClean := len(exp) == 0
	cat := func(pass bool) string {
		switch {
		case expectClean && pass:
			return "clean-pass"
		case expectClean:
			return "clean-fail"
		case pass:
			return "error-match"
		}
		return "error-mismatch"
	}
	set := func(pass bool, detail string) TierResult {
		if pass {
			detail = ""
		}
		return TierResult{Pass: pass, Category: cat(pass), Detail: detail}
	}

	// loose
	out[TierLoose] = set((len(exp) > 0) == (len(act) > 0), looseDetail(exp, act))

	// code sets
	expCodes := codeSet(exp, false)
	actCodes := codeSet(act, true) // TS codes only
	nonTS := 0
	for _, d := range act {
		if !d.isTS() {
			nonTS++
		}
	}

	if expectClean {
		clean := len(act) == 0
		d := cleanDetail(act)
		out[TierCodesSuperset] = set(clean, d)
		out[TierCodesExact] = set(clean, d)
		out[TierLineCodeExact] = set(clean, d)
		out[TierFullExact] = set(clean, d)
		return out
	}

	missingCodes := setDiff(expCodes, actCodes)
	extraCodes := setDiff(actCodes, expCodes)
	out[TierCodesSuperset] = set(len(act) > 0 && len(missingCodes) == 0,
		fmt.Sprintf("missing codes %s", strings.Join(missingCodes, ",")))
	exactOK := len(missingCodes) == 0 && len(extraCodes) == 0 && nonTS == 0
	d := fmt.Sprintf("missing codes [%s] extra codes [%s]", strings.Join(missingCodes, ","), strings.Join(extraCodes, ","))
	if nonTS > 0 {
		d += fmt.Sprintf(" (%d diagnostics without a TS code)", nonTS)
	}
	out[TierCodesExact] = set(exactOK, d)

	lcExp := multiset(exp, func(x Diag) string { return fmt.Sprintf("%d:%s", x.Line, x.Code) })
	lcAct := multiset(act, func(x Diag) string { return fmt.Sprintf("%d:%s", x.Line, x.Code) })
	miss, extra := multisetDiff(lcExp, lcAct)
	out[TierLineCodeExact] = set(len(miss) == 0 && len(extra) == 0, diffDetail(miss, extra))

	fullKey := func(x Diag) string {
		return fmt.Sprintf("%d:%d:%s:%s", x.Line, x.Col, x.Code, strings.TrimSpace(firstLine(x.Msg)))
	}
	fExp := multiset(exp, fullKey)
	fAct := multiset(act, fullKey)
	fm, fe := multisetDiff(fExp, fAct)
	out[TierFullExact] = set(len(fm) == 0 && len(fe) == 0, diffDetail(fm, fe))
	return out
}

func looseDetail(exp, act []Diag) string {
	if len(exp) > 0 {
		return fmt.Sprintf("expected %d errors, got none", len(exp))
	}
	return cleanDetail(act)
}

// cleanDetail summarises the first few diagnostics of a test that should have been clean.
func cleanDetail(act []Diag) string {
	var parts []string
	for i, d := range act {
		if i >= 3 {
			parts = append(parts, fmt.Sprintf("... and %d more", len(act)-3))
			break
		}
		parts = append(parts, fmt.Sprintf("%d:%d %s %s", d.Line, d.Col, d.Code, firstLine(d.Msg)))
	}
	return strings.Join(parts, "; ")
}

func diffDetail(missing, extra []string) string {
	cut := func(xs []string) string {
		if len(xs) > 6 {
			return strings.Join(xs[:6], " ") + fmt.Sprintf(" ...+%d", len(xs)-6)
		}
		return strings.Join(xs, " ")
	}
	return fmt.Sprintf("missing [%s] extra [%s]", cut(missing), cut(extra))
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}

func codeSet(ds []Diag, tsOnly bool) map[string]bool {
	s := make(map[string]bool)
	for _, d := range ds {
		if d.Code == "" || (tsOnly && !d.isTS()) {
			continue
		}
		s[d.Code] = true
	}
	return s
}

// setDiff returns the sorted members of a that are not in b.
func setDiff(a, b map[string]bool) []string {
	var out []string
	for k := range a {
		if !b[k] {
			out = append(out, k)
		}
	}
	sort.Strings(out)
	return out
}

func multiset(ds []Diag, key func(Diag) string) map[string]int {
	m := make(map[string]int, len(ds))
	for _, d := range ds {
		m[key(d)]++
	}
	return m
}

// multisetDiff returns the keys (repeated per surplus count, sorted) that exp has more of than act (missing) and vice versa (extra).
func multisetDiff(exp, act map[string]int) (missing, extra []string) {
	for k, n := range exp {
		for i := act[k]; i < n; i++ {
			missing = append(missing, k)
		}
	}
	for k, n := range act {
		for i := exp[k]; i < n; i++ {
			extra = append(extra, k)
		}
	}
	sort.Strings(missing)
	sort.Strings(extra)
	return missing, extra
}
