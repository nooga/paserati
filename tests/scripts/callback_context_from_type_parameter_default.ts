// Callbacks in an object literal argument whose type parameters nothing
// infers are typed by the parameters' defaults, and the result takes them.
// expect: 1
interface Tx<I, O> {
  transform?: (chunk: I, controller: { enqueue(chunk: O): void }) => void;
}
interface Sx<I = any, O = any> { readonly i?: I; readonly o?: O }
const Sx: { new <I = any, O = any>(t?: Tx<I, O>): Sx<I, O> } = class { constructor(t?: any) {} } as any;
const out: string[] = [];
const s: Sx<any, any> = new Sx({
  transform(chunk, controller) {
    out.push(chunk);
    controller.enqueue(chunk);
  },
});
1;
