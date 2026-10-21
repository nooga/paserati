// A function whose only returns are bare `return;` returns void (Promise<void>
// when async), so it fits a `void | PromiseLike<void>` callback slot.
// expect: done
declare const flag: boolean;
async function stop() {
  if (flag) {
    return;
  }
}
// Promise<void>, not Promise<undefined>: void does not fit undefined.
const isVoid: Promise<void> extends ReturnType<typeof stop> ? 1 : 0 = 1;
function maybe(n: number) {
  if (n > 0) return n;
  return;
}
const m: number | undefined = maybe(1);
const pull: () => void | PromiseLike<void> = async () => {
  if (flag) return;
};
// The overload taking a default source accepts it.
const stream = new ReadableStream({
  async pull(controller) {
    if (flag) {
      return;
    }
    controller.enqueue(1);
  },
});
"done";
