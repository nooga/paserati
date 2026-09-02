// expect: 42|true|43|7|1,2|5|8,9|3|a,b,1,2
// Issue #194: a C-style for-init that starts with a member/bracket/paren/this
// expression must keep parsing past LESSGREATER precedence: `=`, compound
// assignment, ||, &&, ??, ==, |, ?:, and the comma operator. Previously `=`
// silently discarded its RHS and the rest were "';' expected" parse errors.
let e: any = { body: null, x: 0, y: 0, k: null };
let out: string[] = [];

for (e.x = 42; false; ) {}
out.push(String(e.x));

for (e.body || (e.body = []); false; ) {}
out.push(String(Array.isArray(e.body)));

for (e.x += 1; false; ) {}
out.push(String(e.x));

for (e.y ||= 7; false; ) {}
out.push(String(e.y));

// Operators below LESSGREATER that are not assignments must still parse.
for (e.x && e.y; false; ) {}
for (e.x ?? 0; false; ) {}
for (e.x == 43; false; ) {}
for (e.x === 43; false; ) {}
for (e.x != 43; false; ) {}
for (e.x !== 43; false; ) {}
for (e.x | 1; false; ) {}
for (e.x ^ 1; false; ) {}
for (e.x & 1; false; ) {}
for (e.x ? 1 : 2; false; ) {}
for ((1) || 1; false; ) {}
for ([1, 2].length || 1; false; ) {}
for (({} as any).x || 1; false; ) {}

for (e.x = 1, e.y = 2; false; ) {}
out.push(e.x + "," + e.y);

for (e["x"] = 5; false; ) {}
out.push(String(e.x));

let arr: number[] = [0, 0];
for ([arr[0], arr[1]] = [8, 9]; false; ) {}
out.push(arr[0] + "," + arr[1]);

class C {
  n: number = 0;
  run(): number {
    for (this.n = 3; false; ) {}
    for (this.n || (this.n = 4); false; ) {}
    return this.n;
  }
}
out.push(String(new C().run()));

// for-in / for-of over a member-expression target still work.
let seen: string[] = [];
for (e.k in { a: 1, b: 2 }) seen.push(e.k);
for (e.k of [1, 2]) seen.push(String(e.k));
out.push(seen.join(","));

out.join("|");
