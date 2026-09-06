// Regression test for issue #285: a method invoked via tail call (from a
// non-method function) must retain its [[HomeObject]] so super.x still works.
// expect: base-parse
class Base {
  parse(): string {
    return "base-parse";
  }
}
class Derived extends Base {
  parse(): string {
    return super.parse();
  }
}
function run(p: Derived): string {
  return p.parse(); // tail call position, but p.parse is not itself a method here
}
run(new Derived());
