package builtins

import "testing"

// The invalid tags are test262/harness/testIntl.js's getInvalidLanguageTags
// list; each must be rejected by IsStructurallyValidLanguageTag.
func TestIntlStructurallyInvalidLanguageTags(t *testing.T) {
	invalid := []string{
		"", "i", "x", "u", "419", "u-nu-latn-cu-bob", "hans-cmn-cn", "abcdefghi",
		"cmn-hans-cn-u-u", "cmn-hans-cn-t-u-ca-u", "de-gregory-gregory", "*", "de-*",
		"中文", "en-ß", "ıd", "es-Latn-latn", "pl-PL-pl", "u-ca-gregory", "de-1996-1996",
		"pt-u-ca-gregory-u-nu-latn",
		"no-nyn", "i-klingon", "zh-hak-CN", "sgn-ils", "x-foo", "x-en-US-12345",
		"x-12345-12345-en-US", "x-en-US-12345-12345", "x-en-u-foo", "x-en-u-foo-u-bar", "x-u-foo",
		"de_DE", "DE_de", "cmn_Hans", "cmn-hans_cn", "es_419", "es-419-u-nu-latn-cu_bob",
		"i_klingon", "cmn-hans-cn-t-ca-u-ca-x_t-u", "enochian_enochian", "de-gregory_u-ca-gregory",
		"en\x00", " en", "en ", "it-IT-Latn", "de-u", "de-u-", "de-u-ca-", "de-u-ca-gregory-",
		"si-x", "x-", "x-y-", "en-GB-oed", "x-private", "root",
	}
	for _, tag := range invalid {
		if intlIsStructurallyValidLanguageTag(tag) {
			t.Errorf("%q should be structurally invalid", tag)
		}
	}
}

func TestIntlStructurallyValidLanguageTags(t *testing.T) {
	valid := []string{
		"en", "EN", "en-US", "en-us", "sr-Thai-RS", "zh-CN", "und", "zxx", "xyz",
		"en-u-ca-gregory", "en-US-u-nu-latn", "cmn-Hans-CN", "sl-rozaj-biske-1994",
		"en-a-bbb-x-a-ccc", "de-x-abc", "en-Latn", "zh-hans-cn-t-zh-hant-tw",
		"en-u-foo-bar-nu-thai-ca-buddhist-kk-true", "de-DE-u-co-phonebk", "en-US-x-twain",
		"en-t-k0-dvorak", "en-u-attr", "de-1996", "es-419", "abcdefgh", "en-0-abc", "en-u-nu",
	}
	for _, tag := range valid {
		if !intlIsStructurallyValidLanguageTag(tag) {
			t.Errorf("%q should be structurally valid", tag)
		}
	}
}

func TestIntlCanonicalizeUnicodeLocaleID(t *testing.T) {
	cases := map[string]string{
		"EN":                                "en",
		"en-us":                             "en-US",
		"EN-LATN-us":                        "en-Latn-US",
		"sr-thai-rs":                        "sr-Thai-RS",
		"iw":                                "he",
		"in-ID":                             "id-ID",
		"sh":                                "sr-Latn",
		"sh-Cyrl":                           "sr-Cyrl",
		"sl-1994-rozaj-biske":               "sl-1994-biske-rozaj",
		"en-u-foo-bar-nu-thai-ca-buddhist":  "en-u-bar-foo-ca-buddhist-nu-thai",
		"en-u-kk-true":                      "en-u-kk",
		"en-u-ca-gregory-ca-islamic":        "en-u-ca-gregory",
		"en-x-Twain-U":                      "en-x-twain-u",
		"en-t-k0-dvorak-h0-hybrid":          "en-t-h0-hybrid-k0-dvorak",
		"en-t-ZH-HANT-TW":                   "en-t-zh-hant-tw",
		"en-Z-abc-A-def-U-ca-gregory-x-foo": "en-a-def-u-ca-gregory-z-abc-x-foo",
		"de-u-co-phonebk-x-PRIV":            "de-u-co-phonebk-x-priv",
	}
	for in, want := range cases {
		if !intlIsStructurallyValidLanguageTag(in) {
			t.Errorf("%q unexpectedly invalid", in)
			continue
		}
		if got := intlCanonicalizeUnicodeLocaleID(in); got != want {
			t.Errorf("canonicalize(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestIntlLookupMatcher(t *testing.T) {
	cases := []struct {
		requested []string
		want      string
	}{
		{[]string{"en-US"}, "en-US"},
		{[]string{"xyz", "ar"}, "ar"},
		{[]string{"en-US-u-nu-latn"}, "en-US"},
		{[]string{"en-US-x-twain"}, "en-US"},
		{[]string{"sl-rozaj-biske-1994"}, "sl"},
		{[]string{"sr-Thai-RS"}, "sr-Thai-RS"},
		{[]string{"zxx"}, intlDefaultLocale()},
		{nil, intlDefaultLocale()},
	}
	for _, c := range cases {
		if got := intlLookupMatcher(c.requested); got != c.want {
			t.Errorf("lookup(%v) = %q, want %q", c.requested, got, c.want)
		}
	}
	if got := intlLookupSupportedLocales([]string{"sr-Thai-RS", "zxx", "de", "en-u-nu-latn", "xyz"}); len(got) != 3 ||
		got[0] != "sr-Thai-RS" || got[1] != "de" || got[2] != "en-u-nu-latn" {
		t.Errorf("supported = %v", got)
	}
}

func TestIntlNormalizeWTF8(t *testing.T) {
	// A surrogate pair spelled as two WTF-8 surrogates (ED A0 80 ED B0 80)
	// is joined into the 4-byte UTF-8 form of U+10000; a lone surrogate stays.
	if got := intlNormalizeWTF8("\xed\xa0\x80\xed\xb0\x80"); got != "\U00010000" {
		t.Errorf("normalize(pair) = %q", got)
	}
	if got := intlNormalizeWTF8("a\xed\xa0\x80b"); got != "a\xed\xa0\x80b" {
		t.Errorf("normalize(lone) = %q", got)
	}
	if s := "plain 😀 ퟿"; intlNormalizeWTF8(s) != s {
		t.Errorf("normalize changed valid UTF-8")
	}
}

func TestIntlSanitizeWTF8(t *testing.T) {
	// U+D800 lone lead surrogate in WTF-8 is ED A0 80.
	in := "a\xed\xa0\x80b"
	got := intlSanitizeWTF8(in, intlSurrogateStandInOther)
	if got != "a�b" || len(got) != len(in) {
		t.Errorf("sanitize = %q", got)
	}
	// Valid UTF-8 (including U+D7FF = ED 9F BF, the last pre-surrogate code
	// point, and 4-byte astral characters) is returned untouched.
	for _, s := range []string{"plain", "퟿", "😀", "é"} {
		if intlSanitizeWTF8(s, intlSurrogateStandInOther) != s {
			t.Errorf("sanitize(%q) changed valid UTF-8", s)
		}
	}
}
