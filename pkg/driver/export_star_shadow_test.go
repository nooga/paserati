package driver

import (
	"path/filepath"
	"testing"
)

// TestExplicitExportShadowsStarExport checks that a module's own exports win
// over names from `export * from`, wherever they appear in the source
// (paserati#562). Before, an explicit export after the star export threw
// "Cannot access ... before initialization".
func TestExplicitExportShadowsStarExport(t *testing.T) {
	cases := []struct {
		name, facade, want string
	}{
		{"local after star", "export * from \"./i.mjs\";\nexport const b = 20;\n", `{"a":1,"b":20}|20`},
		{"local specifier after star", "export * from \"./i.mjs\";\nconst b = 20; export { b };\n", `{"a":1,"b":20}|20`},
		{"local before star", "export const b = 20;\nexport * from \"./i.mjs\";\n", `{"a":1,"b":20}|20`},
		{"indirect after star", "export * from \"./i.mjs\";\nexport { b } from \"./j.mjs\";\n", `{"a":1,"b":200}|200`},
		{"namespace after star", "export * from \"./i.mjs\";\nexport * as b from \"./j.mjs\";\n", `{"a":1,"b":{"b":200}}|{"b":200}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			mustWrite(t, filepath.Join(dir, "i.mjs"), "export const a = 1; export const b = 2;\n")
			mustWrite(t, filepath.Join(dir, "j.mjs"), "export const b = 200;\n")
			mustWrite(t, filepath.Join(dir, "f.mjs"), tc.facade)
			mustWrite(t, filepath.Join(dir, "m.mjs"),
				"import * as ns from \"./f.mjs\";\n"+
					"import { b } from \"./f.mjs\";\n"+
					"JSON.stringify(ns) + \"|\" + JSON.stringify(b);\n")
			if got := runFileForValue(t, dir, "m.mjs"); got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}
