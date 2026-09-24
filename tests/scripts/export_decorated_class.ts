// expect: true function
function tag<T>(target: T): T { (target as any).tagged = true; return target; }
export
@tag
class C {}
export @tag abstract class D {}
`${(C as any).tagged} ${typeof D}`;
