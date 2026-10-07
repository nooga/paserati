package lexer

import "testing"

// Like tsc's scanner, string/template/number scanning reports a problem as a
// diagnostic on the token and keeps going, instead of producing ILLEGAL.
func TestScannerDiagnostics(t *testing.T) {
	cases := []struct {
		src      string
		wantType TokenType
		wantCode string
		tmpl     bool // diagnostics are on the template string token
	}{
		{`"\u{110000}"`, STRING, "TS1198", false},
		{`"\u{67"`, STRING, "TS1199", false},
		{`"\u12"`, STRING, "TS1125", false},
		{`"\xg"`, STRING, "TS1125", false},
		{"\"abc\n", STRING, "TS1002", false},
		{`1e`, NUMBER, "TS1124", false},
		{`0x`, NUMBER, "TS1125", false},
		{`0b`, NUMBER, "TS1177", false},
	}
	for _, tc := range cases {
		l := NewLexer(tc.src)
		tok := l.NextToken()
		if tok.Type != tc.wantType {
			t.Errorf("%q: token type %s, want %s", tc.src, tok.Type, tc.wantType)
			continue
		}
		if len(tok.Diags) != 1 || tok.Diags[0].Code != tc.wantCode {
			t.Errorf("%q: diagnostics %+v, want one %s", tc.src, tok.Diags, tc.wantCode)
		}
	}

	// A template string carries its escape diagnostics separately (they only
	// apply to untagged templates).
	l := NewLexer("`\\u{hello} \\xtra`")
	l.NextToken() // TEMPLATE_START
	tok := l.NextToken()
	// `\u{hello}` also yields TS1199 at the same position (tsc's scanner does too;
	// the parser keeps only the first error per position), then `\xtra` TS1125.
	if tok.Type != TEMPLATE_STRING || !tok.CookedIsUndefined || len(tok.TemplateDiags) != 3 {
		t.Fatalf("template string token = %+v", tok)
	}
	if tok.TemplateDiags[0].Code != "TS1125" || tok.TemplateDiags[2].Code != "TS1125" {
		t.Errorf("template diagnostics = %+v", tok.TemplateDiags)
	}

	l = NewLexer("`abc")
	l.NextToken()
	tok = l.NextToken()
	if tok.Type != ILLEGAL || len(tok.Diags) != 1 || tok.Diags[0].Code != "TS1160" {
		t.Errorf("unterminated template token = %+v", tok)
	}
}
