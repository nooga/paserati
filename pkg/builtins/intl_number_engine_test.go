package builtins

import "testing"

func TestIntlRoundingModes(t *testing.T) {
	cases := []struct {
		in   string
		fd   int
		mode string
		want string
	}{
		{"1.005", 2, "halfExpand", "1.01"},
		{"-1.005", 2, "halfExpand", "-1.01"},
		{"2.5", 0, "halfEven", "2"},
		{"3.5", 0, "halfEven", "4"},
		{"-2.5", 0, "halfCeil", "-2"},
		{"-2.5", 0, "halfFloor", "-3"},
		{"2.1", 0, "ceil", "3"},
		{"-2.1", 0, "ceil", "-2"},
		{"2.9", 0, "trunc", "2"},
		{"2.5", 0, "halfTrunc", "2"},
		{"0.0001", 2, "expand", "0.01"},
	}
	for _, c := range cases {
		d, ok := intlDecimalFromString(c.in)
		if !ok {
			t.Fatalf("parse %q", c.in)
		}
		raw, _ := d.toRawFixed(0, c.fd, 1, c.mode)
		got := raw.intDigits
		if raw.fracDigits != "" {
			got += "." + raw.fracDigits
		}
		if d.neg {
			got = "-" + got
		}
		if got != c.want {
			t.Errorf("%s fd=%d %s: got %s, want %s", c.in, c.fd, c.mode, got, c.want)
		}
	}
}

func TestIntlRoundingIncrementAndPrecision(t *testing.T) {
	d, _ := intlDecimalFromString("1.23")
	raw, _ := d.toRawFixed(2, 2, 5, "halfExpand")
	if raw.intDigits+"."+raw.fracDigits != "1.25" {
		t.Errorf("increment 5: got %s.%s", raw.intDigits, raw.fracDigits)
	}
	d, _ = intlDecimalFromString("9.996")
	raw, _ = d.toRawPrecision(1, 3, "halfExpand")
	if raw.intDigits != "10" || raw.fracDigits != "" {
		t.Errorf("precision carry: got %s.%s", raw.intDigits, raw.fracDigits)
	}
	raw, _ = intlDecimalFromFloat(0).toRawPrecision(3, 3, "halfExpand")
	if raw.intDigits != "0" || raw.fracDigits != "00" {
		t.Errorf("zero precision: got %s.%s", raw.intDigits, raw.fracDigits)
	}
}

func TestIntlDecimalParsing(t *testing.T) {
	for in, ok := range map[string]bool{"  12.5e3 ": true, "0x1F": true, "1_000": false, ".5": true, "5.": true, "e5": false, "-Infinity": true, "": true} {
		if _, got := intlDecimalFromString(in); got != ok {
			t.Errorf("%q: ok=%v, want %v", in, got, ok)
		}
	}
	d, _ := intlDecimalFromString("12345678901234567890.5")
	raw, _ := d.toRawFixed(0, 1, 1, "halfExpand")
	if raw.intDigits != "12345678901234567890" || raw.fracDigits != "5" {
		t.Errorf("exact string value lost: %s.%s", raw.intDigits, raw.fracDigits)
	}
}
