// no-typecheck
// Test ECMAScript private field/method/accessor brand-check semantics:
// - PrivateFieldAdd (initialization) throws if the entry already exists.
// - PrivateFieldSet (assignment) throws if the entry does NOT exist.
// - PrivateMethodOrAccessorAdd throws if the entry already exists.
// expect: all private brand checks passed

// --- Double construction via a base constructor that returns a pre-existing object ---
// Per spec, re-running InitializeInstanceFields on the same object must throw.

class Base {
  constructor(o: any) {
    return o;
  }
}

class WithPrivateField extends Base {
  #x = 1;
}

let shared1 = {};
new WithPrivateField(shared1);
let fieldDoubleInitThrew = false;
try {
  new WithPrivateField(shared1);
} catch (e) {
  fieldDoubleInitThrew = e instanceof TypeError;
}
if (!fieldDoubleInitThrew) throw new Error("expected TypeError on double private field init");

class WithPrivateMethod extends Base {
  #m() {
    return 1;
  }
}

let shared2 = {};
new WithPrivateMethod(shared2);
let methodDoubleInitThrew = false;
try {
  new WithPrivateMethod(shared2);
} catch (e) {
  methodDoubleInitThrew = e instanceof TypeError;
}
if (!methodDoubleInitThrew) throw new Error("expected TypeError on double private method init");

class WithPrivateAccessor extends Base {
  get #p() {
    return 1;
  }
  set #p(v: any) {}
}

let shared3 = {};
new WithPrivateAccessor(shared3);
let accessorDoubleInitThrew = false;
try {
  new WithPrivateAccessor(shared3);
} catch (e) {
  accessorDoubleInitThrew = e instanceof TypeError;
}
if (!accessorDoubleInitThrew) throw new Error("expected TypeError on double private accessor init");

// --- Assigning to a private field on an object that never had it declared ---
// Per spec, PrivateFieldSet must throw when the entry is missing - it must
// never silently create the field.

class HasPrivateField {
  #field: number = 0;
  setDirect() {
    this.#field = 1;
  }
  setViaArrayDestructure() {
    [this.#field] = [1];
  }
  setViaObjectDestructure() {
    ({ a: this.#field } = { a: 1 });
  }
  setViaSpread() {
    ({ ...this.#field } = {});
  }
}

let proto: any = HasPrivateField.prototype;

let directThrew = false;
try {
  proto.setDirect.call({});
} catch (e) {
  directThrew = e instanceof TypeError;
}
if (!directThrew) throw new Error("expected TypeError assigning private field to foreign object");

let arrayDestructureThrew = false;
try {
  proto.setViaArrayDestructure.call({});
} catch (e) {
  arrayDestructureThrew = e instanceof TypeError;
}
if (!arrayDestructureThrew)
  throw new Error("expected TypeError destructuring-assigning private field (array) to foreign object");

let objectDestructureThrew = false;
try {
  proto.setViaObjectDestructure.call({});
} catch (e) {
  objectDestructureThrew = e instanceof TypeError;
}
if (!objectDestructureThrew)
  throw new Error("expected TypeError destructuring-assigning private field (object) to foreign object");

let spreadThrew = false;
try {
  proto.setViaSpread.call({});
} catch (e) {
  spreadThrew = e instanceof TypeError;
}
if (!spreadThrew) throw new Error("expected TypeError object-spread-assigning private field to foreign object");

// --- Regression guard: normal (same-instance) private field/method usage still works ---

class Counter {
  #count = 0;
  #step() {
    this.#count++;
  }
  bump(): number {
    this.#step();
    [this.#count] = [this.#count + 1];
    return this.#count;
  }
}

let counter = new Counter();
if (counter.bump() !== 2) throw new Error("normal private field/method usage regressed");

"all private brand checks passed";
