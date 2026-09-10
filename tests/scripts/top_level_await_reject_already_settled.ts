// expect: caught:caught-me,caught-settled:late
// Companion to top_level_await_rejection.ts (the uncaught case) and
// queue_microtask.ts/readable_stream_async_close_drain.ts (#393's
// top-level-await-must-always-defer-through-a-microtask fix in vm.go's
// OpAwait): a top-level `await` of an ALREADY-rejected promise, inside a
// try/catch, must still reach the catch block - both for a freshly rejected
// promise and for one that settled well before the await runs (#102 is
// exactly this regressing for the fresh case; the settled-before-await case
// is the one the top-level-await/queueMicrotask-ordering fix changed the
// internal path for, since it now resumes via a deferred reaction instead
// of reading the promise's state directly).
const log: string[] = [];

try {
  await Promise.reject(new Error("caught-me"));
} catch (e) {
  log.push("caught:" + (e as Error).message);
}

const p = Promise.reject(new Error("late"));
// Give `p` plenty of time to settle (it already has, synchronously) before
// we ever await it.
await new Promise<void>((resolve) => {
  queueMicrotask(() => resolve());
});
try {
  await p;
} catch (e) {
  log.push("caught-settled:" + (e as Error).message);
}

log.join(",");
