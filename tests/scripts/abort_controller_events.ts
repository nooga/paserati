// expect: true
// Regression test for https://github.com/nooga/paserati/issues/372:
// AbortController.abort() must synchronously dispatch "abort" to every
// addEventListener listener (respecting `once`) and to onabort,
// removeEventListener must actually unregister, and AbortSignal.any() must
// propagate a signal that aborts *after* any() was called, not just one
// that was already aborted.
const checks: boolean[] = [];

// addEventListener listeners fire, with the right Event shape.
{
  const c = new AbortController();
  let seenType = "";
  let seenTarget: any = null;
  c.signal.addEventListener("abort", (e: any) => {
    seenType = e.type;
    seenTarget = e.target;
  });
  c.abort("boom");
  checks.push(seenType === "abort");
  checks.push(seenTarget === c.signal);
  checks.push(c.signal.aborted === true);
  checks.push(c.signal.reason === "boom");
}

// `once` listeners run exactly once.
{
  const c = new AbortController();
  let count = 0;
  c.signal.addEventListener("abort", () => { count++; }, { once: true });
  c.abort();
  c.abort(); // second abort() is a no-op - already aborted
  checks.push(count === 1);
}

// removeEventListener actually unregisters the listener.
{
  const c = new AbortController();
  let called = false;
  const handler = () => { called = true; };
  c.signal.addEventListener("abort", handler);
  c.signal.removeEventListener("abort", handler);
  c.abort();
  checks.push(called === false);
}

// A listener removed by an earlier listener in the same dispatch doesn't fire.
{
  const c = new AbortController();
  let secondCalled = false;
  const second = () => { secondCalled = true; };
  c.signal.addEventListener("abort", () => {
    c.signal.removeEventListener("abort", second);
  });
  c.signal.addEventListener("abort", second);
  c.abort();
  checks.push(secondCalled === false);
}

// onabort fires alongside addEventListener listeners.
{
  const c = new AbortController();
  let fired = false;
  c.signal.onabort = () => { fired = true; };
  c.abort();
  checks.push(fired === true);
}

// AbortSignal.any() reflects a source that's already aborted.
{
  const a = new AbortController();
  const b = new AbortController();
  a.abort("first");
  const combined = AbortSignal.any([a.signal, b.signal]);
  checks.push(combined.aborted === true);
  checks.push(combined.reason === "first");
}

// AbortSignal.any() propagates a source that aborts *later*.
{
  const a = new AbortController();
  const b = new AbortController();
  const combined = AbortSignal.any([a.signal, b.signal]);
  let fired = false;
  combined.addEventListener("abort", () => { fired = true; });
  checks.push(combined.aborted === false);
  b.abort("second");
  checks.push(combined.aborted === true);
  checks.push(combined.reason === "second");
  checks.push(fired === true);
}

// throwIfAborted throws the actual reason value, not a wrapper around it.
{
  const c = new AbortController();
  const reason = { code: 42 };
  c.abort(reason as any);
  let thrown: any = null;
  try {
    c.signal.throwIfAborted();
  } catch (e) {
    thrown = e;
  }
  checks.push(thrown === reason);
}

checks.every((v) => v === true);
