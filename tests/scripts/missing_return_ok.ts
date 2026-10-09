// expect: 3
function a(x: boolean): number { if (x) return 1; else return 2; }
function b(): number { while (true) { return 1; } }
function c(x: 1 | 2): number { switch (x) { case 1: return 1; case 2: return 2; } }
function d(): number { try { return 1; } finally { } }
function e(): number | undefined { }
function g(): number { throw new Error("x"); }
a(true) + a(false);
