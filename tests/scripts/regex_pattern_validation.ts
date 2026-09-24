// expect: true|true|true|true|true|true|true|true|true|SyntaxError|SyntaxError|SyntaxError|SyntaxError
// Valid patterns - including Annex B forms outside Unicode mode and the
// duplicate-named-groups rule - are accepted, and the RegExp constructor
// rejects exactly what a literal would reject, as a runtime SyntaxError.
let out: string[] = [];
out.push(String(/a{/.test("a{")));             // lone '{' is literal without u
out.push(String(/\8/.test("8")));              // identity escape
out.push(String(/(?=a)*b/.test("b")));         // quantified lookahead without u
out.push(String(/(?<y>\d+)-x|(?<y>x)/.test("x"))); // same name, different alternatives
out.push(String(/\p{L}/.test("p{L}")));        // \p is an identity escape without u
out.push(String(/^\p{Script_Extensions=Hira}$/u.test("ー")));
out.push(String(/^\p{Extended_Pictographic}$/u.test("😀")));
out.push(String(/^\p{CWU}$/u.test("ß")));
out.push(String(/^\p{sc=Hluw}$/u.test("𔐀")));

function tryNew(p: string, f: string): string {
    try {
        new RegExp(p, f);
        return "ok";
    } catch (e) {
        return (e as Error).name;
    }
}
out.push(tryNew("(?i-i:a)", ""));
out.push(tryNew("\\p{Foo}", "u"));
out.push(tryNew("a{2,1}", ""));
out.push(tryNew("[\\p{RGI_Emoji}]", "u"));
out.join("|");
