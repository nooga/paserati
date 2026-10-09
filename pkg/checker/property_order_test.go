package checker

import (
	"strings"
	"testing"

	"github.com/nooga/paserati/pkg/lexer"
	"github.com/nooga/paserati/pkg/parser"
	"github.com/nooga/paserati/pkg/types"
)

func checkedAlias(t *testing.T, src, name string) *types.ObjectType {
	t.Helper()
	p := parser.NewParser(lexer.NewLexer(src))
	prog, perrs := p.ParseProgram()
	if len(perrs) > 0 {
		t.Fatalf("parse: %v", perrs)
	}
	c := NewChecker()
	c.Check(prog)
	typ, ok := c.env.ResolveType(name)
	if !ok {
		t.Fatalf("type %s not found", name)
	}
	obj, ok := typ.(*types.ObjectType)
	if !ok {
		t.Fatalf("%s is %T, want object type", name, typ)
	}
	return obj
}

func TestPropertyDeclarationOrder(t *testing.T) {
	src := `
interface Base { id: string; created: string }
interface Item extends Base { name: string; size: number; tags?: string[] }
type Lit = { z: 1; a: 2; m: 3 };
class K { zeta = 1; alpha = 2; mid(): void {} }
`
	cases := map[string]string{
		"Item": "id,created,name,size,tags",
		"Lit":  "z,a,m",
		"K":    "zeta,alpha,mid",
	}
	for name, want := range cases {
		got := strings.Join(checkedAlias(t, src, name).EffectivePropertyNames(), ",")
		if got != want {
			t.Errorf("%s: got %s, want %s", name, got, want)
		}
	}
}

func TestPropertyDocs(t *testing.T) {
	src := `
interface Base {
    /** the id */
    id: string;
}
/** An item. */
interface Item extends Base {
    /**
     * Display name
     * @default x
     */
    name: string;
    plain: number;
}
type Lit = { /** lit doc */ a: 1; b: 2 };
class K {
    /** field doc */
    f = 1;
    /** method doc */
    m(): void {}
    /** getter doc */
    get g(): number { return 1; }
}
`
	item := checkedAlias(t, src, "Item")
	if got := item.PropertyDoc("id"); got != "the id" {
		t.Errorf("inherited doc: %q", got)
	}
	if got := item.PropertyDoc("name"); got != "Display name\n@default x" {
		t.Errorf("name doc: %q", got)
	}
	if item.PropertyDoc("plain") != "" || item.Doc != "An item." {
		t.Errorf("plain=%q Doc=%q", item.PropertyDoc("plain"), item.Doc)
	}
	if got := checkedAlias(t, src, "Lit").PropertyDoc("a"); got != "lit doc" {
		t.Errorf("literal doc: %q", got)
	}
	k := checkedAlias(t, src, "K")
	for name, want := range map[string]string{"f": "field doc", "m": "method doc", "g": "getter doc"} {
		if got := k.PropertyDoc(name); got != want {
			t.Errorf("K.%s doc: got %q want %q", name, got, want)
		}
	}
}
