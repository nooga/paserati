// expect: true

// Test class inheritance inside an IIFE (matches typescript.js pattern)
(() => {
    class Parent {
        value: number;
        constructor(v: number) {
            this.value = v;
        }
        getValue(): number {
            return this.value;
        }
    }

    class Child extends Parent {
        extra: number;
        constructor(v: number, e: number) {
            super(v);
            this.extra = e;
        }
    }

    const c = new Child(10, 20);
    return c.getValue() === 10 && c.extra === 20;
})();
