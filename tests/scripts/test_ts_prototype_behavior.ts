// Test TypeScript prototype behavior - functions have prototype as any

// expect: 10

interface Zupa { foo: number; goo: number }

function zupa(this: Zupa) {
    this.foo = this.goo + 7;
}

// This should work - zupa.prototype has type 'any'
zupa.prototype.goo = 10;