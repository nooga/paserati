// expect: Promise { [1, [2, 3]] }
// Companion to async_rest_param_identity.ts: covers a rest parameter
// alongside preceding fixed parameters, and with extra arguments actually
// present, rather than only the fully-empty case.
async function afMixed(a: number, ...rest: number[]) {
  return [a, rest];
}
afMixed(1, 2, 3);
