// expect: function
// await inside a class `async *` method parses when the class is inside a non-async function (#668)
let t;
(function (exports) { class D { async *g() { const d = await 1; yield d; } }
t = typeof new D().g; })({});
t;
