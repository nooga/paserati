// expect: 1
const an: Awaited<number> = 1;
const z: NoInfer<number> = an;
function w<T>(opts: { p: T; onTimeout: () => NoInfer<T> }): T { return opts.p; }
w({ p: 1, onTimeout: () => an });
