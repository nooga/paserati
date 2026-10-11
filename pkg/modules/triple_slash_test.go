package modules

import (
	"reflect"
	"testing"
)

func TestTripleSlashReferences(t *testing.T) {
	src := `#!/usr/bin/env node
/**
 * License header
 */
/// <reference path="assert.d.ts" />
/// <reference path="../shared/x.d.ts"/>
/// <reference types="node" />
/// <reference lib="es2020" />
// plain comment
declare const a: number;
/// <reference path="late.d.ts" />
`
	got := TripleSlashReferences(src)
	want := []string{"./assert.d.ts", "../shared/x.d.ts", "node"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}
