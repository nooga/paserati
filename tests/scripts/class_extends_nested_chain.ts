// expect: true

// Test nested class inheritance in function scope
function testNestedClasses(): boolean {
    class Base {
        value: number;
        constructor(v: number) {
            this.value = v;
        }
    }

    class Middle extends Base {
        middle: string;
        constructor(v: number, m: string) {
            super(v);
            this.middle = m;
        }
    }

    class Derived extends Middle {
        extra: boolean;
        constructor(v: number, m: string, e: boolean) {
            super(v, m);
            this.extra = e;
        }
    }

    const d = new Derived(42, "hello", true);
    return d.value === 42 && d.middle === "hello" && d.extra === true;
}

testNestedClasses();
