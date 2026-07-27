package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestStrictPropertyInitEnabled(t *testing.T) {
	tests := []struct {
		name       string
		directives TestDirectives
		want       bool
	}{
		{
			// TS 6.0 turned `strict` on by default and regenerated the
			// conformance baselines to match.
			name:       "default on",
			directives: TestDirectives{Raw: map[string]string{}},
			want:       true,
		},
		{
			name:       "strict enables",
			directives: TestDirectives{Raw: map[string]string{"strict": "true"}},
			want:       true,
		},
		{
			name:       "strict false disables",
			directives: TestDirectives{Raw: map[string]string{"strict": "false"}},
			want:       false,
		},
		{
			name:       "explicit strict property init enables",
			directives: TestDirectives{Raw: map[string]string{"strictpropertyinitialization": "true"}},
			want:       true,
		},
		{
			name:       "explicit strict property init overrides strict",
			directives: TestDirectives{Raw: map[string]string{"strict": "true", "strictpropertyinitialization": "false"}},
			want:       false,
		},
		{
			// Multi-value directives generate one baseline per value; the
			// selected baseline is the one carrying errors, so `true` wins.
			name:       "multi-value strict enables",
			directives: TestDirectives{Raw: map[string]string{"strict": "true, false"}},
			want:       true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := strictPropertyInitEnabled(tt.directives); got != tt.want {
				t.Fatalf("strictPropertyInitEnabled() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestLoadExpectedErrorsTreatsZeroSourceErrorsAsClean(t *testing.T) {
	dir := t.TempDir()
	content := `lib.es5.d.ts(--,--): error TS2411: lib diagnostic

==== zeroSource.ts (0 errors) ====
    class C {
        value!: string;
    }
`
	if err := os.WriteFile(filepath.Join(dir, "zeroSource.errors.txt"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	expectErrors, errs := loadExpectedErrors(dir, "zeroSource", TestDirectives{Raw: map[string]string{}})
	if expectErrors {
		t.Fatalf("loadExpectedErrors() expectErrors = true, want false")
	}
	if len(errs) != 0 {
		t.Fatalf("loadExpectedErrors() returned %d errors, want 0", len(errs))
	}
}
