package jsregex

import "testing"

func TestValidate(t *testing.T) {
	valid := []struct{ pattern, flags string }{
		{"", ""},
		{"a|b|", ""},
		{"a{", ""},      // Annex B: a '{' starting no quantifier is literal
		{"a{1", ""},     //
		{"]", ""},       //
		{"}", ""},       //
		{"\\c", ""},     // `\` then literal 'c'
		{"[\\c_]", ""},  // Annex B class control letter
		{"\\8", ""},     // identity escape
		{"\\1", ""},     // legacy octal: no group 1
		{"(a)\\1", "u"}, //
		{"\\k", ""},     // no named groups: identity escape
		{"(?=a)*", ""},  // quantifiable lookahead outside Unicode mode
		{"[\\d-a]", ""}, // class escape range: union outside Unicode mode
		{"\\p{L}", ""},  // identity escape p
		{"\\p{L}", "u"}, //
		{"\\p{gc=Lu}", "u"},
		{"\\p{Script_Extensions=Latn}", "u"},
		{"\\p{RGI_Emoji}", "v"},
		{"[\\p{L}--\\p{Lu}]", "v"},
		{"[\\q{abc|d}&&\\q{d}]", "v"},
		{"[[a-z]&&[aeiou]]", "v"},
		{"\\u{10FFFF}", "u"},
		{"\\uD83D\\uDE00{2}", "u"},
		{"(?<a>x)|(?<a>y)", ""}, // duplicate names in different alternatives
		{"(?<$π>x)\\k<$π>", ""},
		{"(?<\\u{61}>x)", ""},
		{"(?i:a)(?-i:b)(?im-s:c)", ""},
		{"a{2,2}", ""},
		{"a{99999999999999999999,99999999999999999999}", "u"},
	}
	for _, c := range valid {
		if err := Validate(c.pattern, c.flags); err != nil {
			t.Errorf("/%s/%s: unexpected error %v", c.pattern, c.flags, err)
		}
	}

	invalid := []struct{ pattern, flags string }{
		{"(", ""},
		{")", ""},
		{"[", ""},
		{"*", ""},
		{"a**", ""},
		{"{1}", ""},
		{"a{2,1}", ""},
		{"^*", ""},
		{"\\b+", "u"},
		{"(?=a)*", "u"},
		{"(?<=a)?", ""},
		{"]", "u"},
		{"{", "u"},
		{"a{", "u"},
		{"\\c", "u"},
		{"\\8", "u"},
		{"(a)\\2", "u"},
		{"\\00", "u"},
		{"\\-", "u"},
		{"\\u{110000}", "u"},
		{"\\x4", "u"},
		{"[\\d-a]", "u"},
		{"[z-a]", ""},
		{"[😀-😂]", ""}, // code units outside Unicode mode: out of order
		{"\\p{Foo}", "u"},
		{"\\p{ASCII=Y}", "u"},
		{"\\p{ lu}", "u"},
		{"\\p{IsScript=Latin}", "u"},
		{"\\p{RGI_Emoji}", "u"},
		{"\\P{RGI_Emoji}", "v"},
		{"[^\\p{RGI_Emoji}]", "v"},
		{"[^\\q{ab}]", "v"},
		{"[a-z&&b]", "v"},
		{"[a&&&b]", "v"},
		{"[a--b&&c]", "v"},
		{"[(]", "v"},
		{"[a!!b]", "v"},
		{"(?<a>x)(?<a>y)", ""},
		{"(?<a>x)\\k<b>", ""},
		{"(?<a>x)\\k", ""},
		{"\\k<a>", "u"},
		{"(?<1a>x)", ""},
		{"(?<>x)", ""},
		{"(?ii:a)", ""},
		{"(?-:a)", ""},
		{"(?i-i:a)", ""},
		{"(?x:a)", ""},
		{"(?i)", ""},
		{"(?\\u0069:a)", ""},
		{"a", "gg"},
		{"a", "uv"},
		{"a", "x"},
	}
	for _, c := range invalid {
		if err := Validate(c.pattern, c.flags); err == nil {
			t.Errorf("/%s/%s: expected a SyntaxError", c.pattern, c.flags)
		}
	}
}
