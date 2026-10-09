// expect: ok
// Valid programs that #694 briefly rejected (#695).
function uniq(xs: string[]): string[] { return [...new Set(xs)]; }
const seen = new Set<string>();
["a", "b", "a"].forEach((k) => seen.add(k));
const pairs: [string, number][] = [...new Map<string, number>([["a", 1]])];

interface S { id?: string }
class B<T> { state!: T }
class E extends B<S> {
  del(): void { if (!this.state.id) return; this.state.id = undefined; }
}

class Q {
  state: { url?: string } = {};
  get(): string { return "u"; }
  del(u: string): void {}
  run(): void {
    if (!this.state.url) {
      try { this.state.url = this.get(); } catch (e) { return; }
    }
    this.del(this.state.url);
  }
}

declare const describeIt: () => Record<string, unknown> | null;
function fmt(d: Record<string, unknown>): string { return ""; }
function branch(): void {
  const d = describeIt();
  if (!d) { fmt({}); } else { fmt(d); }
}

let email = null;
let emails: string[] = [];
if (uniq(["a"]).length) email = "x";
if (email) emails = [email];

const xs: (string | undefined)[] = ["a", undefined];
const ys: string[] = xs.filter((x): x is string => x !== undefined);

let lazy: string | undefined;
lazy ??= "a";
const forced: string = lazy;

let viaCall: string | undefined;
viaCall = uniq(["q"])[0];
const forced2: string = viaCall;

"ok";
