// expect: true
// paserati#180: a minifier commonly merges `super(...);` with the very next
// statement into one `super(...), <next>;` via the comma operator - legal
// JS (a bare ExpressionStatement's value is always discarded, so this is
// behavior-identical to two separate statements) and exactly what esbuild's
// bundling of glob@13.0.6 produces for one of its derived-class
// constructors. injectFieldInitializers' "find the super() call" search
// only recognized a super() call that IS the entire expression of its
// statement - a comma-joined super() call never matched, so a derived
// class with an instance field always fell into the "no super() call
// found" fallback and prepended the field initializer at the very start
// of the constructor, before 'this' exists at all - throwing "Must call
// super constructor..." even though the source does call super() first.
class Base {
  constructor(t: number) {
    this.baseVal = t;
  }
  baseVal: number;
}

class Derived extends Base {
  field = "field-init-value";
  own = 0;
  constructor(t: number) {
    // The comma here is the exact shape that broke: super() and the
    // following assignment merged into one statement.
    super(t), (this.own = t + 1);
  }
}

const d = new Derived(41);
d.baseVal === 41 && d.field === "field-init-value" && d.own === 42;
