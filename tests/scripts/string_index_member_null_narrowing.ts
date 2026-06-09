// expect: 1
type Bag = {
    other: number | null;
    [index: string]: number | null;
};

let value: Bag = { foo: 1, other: null, bar: null };

if (value.foo !== null) {
    value.foo.toExponential();
    let other: number | null = value.other;
    let bar: number | null = value.bar;
}

1;
