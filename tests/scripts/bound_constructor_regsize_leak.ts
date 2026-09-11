// Regression for #400 (B1: frame/register lifetime): `new` on a bound
// constructor pushed its frame without setting newFrame.allocatedRegSize,
// so that stale frame-slot leftover value (not requiredRegs) was reclaimed
// on return - permanently leaking (or over-reclaiming) registers on every
// construction through a bound function. Confirmed fixed against the real
// tsc.js/lib.dom.d.ts repro (see #399). See docs/runtime-production-roadmap.md#b1.
//
// A handful of iterations is enough: vm.checkRegWindowRelease (#402 B4
// invariant checker) catches a wrong reclaim size on the very first bad
// return, rather than needing tens of thousands of iterations to exhaust
// the (pre-B4) fixed-size register file - this test used to need 60,000
// iterations to overflow and fail; now one is enough, and it will keep
// catching a regression even once B4 removes that fixed ceiling.
// expect: 5
let count = 0;

function Big(this: any) {
  let b1 = 1, b2 = 2, b3 = 3, b4 = 4, b5 = 5, b6 = 6, b7 = 7, b8 = 8, b9 = 9, b10 = 10;
  let b11 = 11, b12 = 12, b13 = 13, b14 = 14, b15 = 15, b16 = 16, b17 = 17, b18 = 18, b19 = 19, b20 = 20;
  let b21 = 21, b22 = 22, b23 = 23, b24 = 24, b25 = 25, b26 = 26, b27 = 27, b28 = 28, b29 = 29, b30 = 30;
  this.a = b1 + b30;
  count++;
}

const Bound = Big.bind(null);

for (let i = 0; i < 5; i++) {
  new (Bound as any)();
}

count;
