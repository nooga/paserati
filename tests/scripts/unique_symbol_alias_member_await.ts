// expect: function
// A unique symbol declared after a type alias names a member of the
// alias's type literal (no TS2304), and awaiting an intersection with a
// Promise yields the promise's value.
type Tagged<TData, TShape> = Promise<TData> & { readonly [tag]: TShape };
const tag: unique symbol = Symbol();
function mk(n: number): Tagged<number, string> {
  return Object.assign(Promise.resolve(n), { [tag]: "s" }) as Tagged<number, string>;
}
let out = 0;
async function run() { const x: number = await mk(42); out = x; }
run();
typeof mk(1).then;
