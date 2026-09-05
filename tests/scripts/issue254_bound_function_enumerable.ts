// expect: true
// paserati#254: a bound function's own properties (set via direct assignment,
// not just Object.assign) were readable via property access/hasOwnProperty
// but invisible to Object.keys and for-in - OpGetOwnKeys had no TypeBoundFunction
// case at all, and OpIn's TypeBoundFunction case never consulted the bound
// function's own Properties table, so for-in's per-key re-check silently
// dropped every key OpGetOwnKeys would otherwise have found.
const checks: boolean[] = [];

function base() {}
const bound = base.bind(undefined);
(bound as any).x = 1;

checks.push(bound.hasOwnProperty("x"));
checks.push((bound as any).x === 1);
checks.push(JSON.stringify(Object.keys(bound)) === '["x"]');

const seen: string[] = [];
for (const k in bound) {
  seen.push(k);
}
checks.push(JSON.stringify(seen) === '["x"]');

checks.every((c) => c === true);
