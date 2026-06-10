// expect: 1
let value: string | undefined = "ok";
if (typeof value === `string`) {
    value.slice(0);
}

let obj: { test: string } | {} = { test: "ok" };
if (`test` in obj) {
    obj.test.slice(0);
}

1;
