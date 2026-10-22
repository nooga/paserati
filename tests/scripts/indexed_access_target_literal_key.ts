// expect: x
interface Roots { ctx: object; errorShape: any; }
function f<T extends Roots>(): T['errorShape'] { return { code: 1 }; }
interface R2 { name: string }
function g<T extends R2>(): T['name'] { return "x"; }
g<R2>();
