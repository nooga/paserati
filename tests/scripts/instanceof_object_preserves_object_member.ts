// expect: 1
let value: string | number | Date = new Date();

if (value instanceof Object) {
    value.getFullYear();
} else if (typeof value === "string") {
    value.length;
} else {
    value.toPrecision(3);
}

1;
