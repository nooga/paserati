// expect: true

// #128: a class declared inside a function body must be visible to every
// closure nested in that function, not just the constructor's own body -
// methods, getters/setters, and further-nested arrow functions all close
// over the class name the same way any other local binding would.
function makeCounter(): boolean {
    class Counter {
        static count: number = 0;

        constructor() {
            // Constructor self-reference (referencing the class from its own body).
            this.ctorSelfRef = Counter;
        }

        // A regular method referencing the class name is itself a nested
        // closure (its own function/compiler), distinct from the constructor.
        inc(): number {
            Counter.count++;
            return Counter.count;
        }

        // An arrow function nested *inside* a method - two closure boundaries
        // away from the class declaration.
        makeIncrementer(): () => number {
            return () => {
                Counter.count++;
                return Counter.count;
            };
        }

        ctorSelfRef: unknown;
    }

    const c = new Counter();
    const ctorOk = c.ctorSelfRef === Counter;
    const methodOk = c.inc() === 1;
    const nestedArrowOk = c.makeIncrementer()() === 2;

    // A sibling closure declared after the class, in the same function scope.
    const siblingReadsClass = () => Counter.count;
    const siblingOk = siblingReadsClass() === 2;

    return ctorOk && methodOk && nestedArrowOk && siblingOk;
}

function makeDerived(): boolean {
    class Base {
        static tag(): string {
            return "base";
        }
    }
    class Derived extends Base {
        static tag(): string {
            return super.tag() + "+derived";
        }
        method(): () => string {
            return () => Derived.tag() + "|" + Base.tag();
        }
    }
    return new Derived().method()() === "base+derived|base";
}

makeCounter() && makeDerived();
