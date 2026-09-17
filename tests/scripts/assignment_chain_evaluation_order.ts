// Parking a chain link's object/key base in a spill slot while the nested
// RHS compiles (the paserati#470 fix) must not change the chain's
// observable evaluation order: references are still resolved left-to-right,
// and stores still happen in the reverse order (innermost/rightmost first),
// exactly as before the base was relocated out of a live register.
// expect: A,B,C|s2=5,s1=5

let log: string[] = [];

let oa: { p?: number } = {};
let ob: { p?: number } = {};
let oc: { p?: number } = {};
let refs = {
  get A() {
    log.push("A");
    return oa;
  },
  get B() {
    log.push("B");
    return ob;
  },
  get C() {
    log.push("C");
    return oc;
  },
};

// Reference resolution order: A, then B, then C (left to right), before the
// final RHS value (99) is even evaluated.
refs.A.p = refs.B.p = refs.C.p = 99;

let calls: string[] = [];
let s1 = {
  set p(v: number) {
    calls.push("s1=" + v);
  },
};
let s2 = {
  set p(v: number) {
    calls.push("s2=" + v);
  },
};

// Store order: s2 (innermost) fires before s1 (outermost) - per spec,
// `s1.p = s2.p = 5` assigns to s2 first, then propagates that same value to
// s1, not the other way around.
s1.p = s2.p = 5;

log.join(",") + "|" + calls.join(",");
