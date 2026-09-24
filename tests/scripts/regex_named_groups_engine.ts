// expect: a|b|y,undefined|x|true|ba|4
// Group names are ECMAScript identifiers (any ID_Start/ID_Continue, '$',
// escapes), which neither engine can spell, and a name may repeat across
// alternatives; the engines see plain numbered groups instead.
let out: string[] = [];
out.push(/(?<π>a)\k<π>/u.exec("aa")![1]);
out.push((/(?<$>a)(?<\u{62}>b)/.exec("ab") as any).groups.b);
let m = /(?<n>y)|(?<n>x)/.exec("y")!;
out.push(m[1] + "," + m[2]);
out.push((/(?<n>y)|(?<n>x)/.exec("x") as any).groups.n);
out.push(String(/\z\A/.test("zA"))); // identity escapes, not engine anchors
out.push("a-b".replace(/(?<x>\w)-(?<y>\w)/, "$<y>$<x>"));
out.push(String(/(?<a>.)(b)(?<c>.)/.exec("abc")!.length));
out.join("|");
