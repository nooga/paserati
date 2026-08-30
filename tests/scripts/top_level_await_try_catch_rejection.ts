// Regression test for #102: a rejected top-level await must be catchable by
// a surrounding try/catch, exactly like inside an async function. Previously
// the top-level-await branch of OpAwait went straight to an unconditional
// "Uncaught (in promise)" error and never consulted the module's exception
// table, so the catch block below was skipped entirely.
// expect: caught:boom,inner-finally,outer:inner,async:boom

const log: string[] = [];

try {
  await Promise.reject(new Error("boom"));
  log.push("not-rejected");
} catch (e) {
  log.push("caught:" + (e as Error).message);
}

try {
  try {
    await Promise.reject(new Error("inner"));
  } finally {
    log.push("inner-finally");
  }
} catch (e) {
  log.push("outer:" + (e as Error).message);
}

async function run() {
  try {
    await Promise.reject(new Error("boom"));
    log.push("async-not-rejected");
  } catch (e) {
    log.push("async:" + (e as Error).message);
  }
}
await run();

log.join(",");
