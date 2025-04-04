// expect: 52

let a: number = 5; // 0b0101
let b: number = 3; // 0b0011

let c = a & b; // c = 0b0001 => 1
let d = a | b; // d = 0b0111 => 7
let e = a ^ b; // e = 0b0110 => 6
// let f = ~a;    // ~5 => -6 (Skipping for now, requires OpBitwiseNot implementation)
let g = a << 1; // g = 0b1010 => 10
let h = a >> 1; // h = 0b0010 => 2

a &= b; // a = a & b => a = 1
b |= 1; // b = b | 1 => b = 3 | 1 => b = 3 (0b0011 | 0b0001 = 0b0011)
c <<= 2; // c = c << 2 => c = 1 << 2 => c = 4
d >>= 1; // d = d >> 1 => d = 7 >> 1 => d = 3 (0b0111 >> 1 = 0b0011)

let k = 10;
let l = 0;
k += 5; // k = 15
l -= 2; // l = -2

let m: boolean = true;
let n: boolean = false;
let o = 100; // temp number

// Temporarily using 'true'/'false' RHS for type compatibility
m &&= false; // m = m && false => m = true && false => m = false
n ||= true; // n = n || true => n = false || true => n = true

let p: number | null = null;
let q: number | undefined = undefined;
let r = 100;
p ??= r; // p = p ?? r => p = null ?? 100 => p = 100
q ??= 50; // q = q ?? 50 => q = undefined ?? 50 => q = 50

let s = 20;
let t = ++s; // s becomes 21, t = 21
let u = s++; // u = 21, s becomes 22
let v = --s; // s becomes 21, v = 21
let w = s--; // w = 21, s becomes 20

// Final calculation based on variables used:
// a=1, b=3, c=4, d=3, e=6, k=15, s=20
// Result = 1 + 3 + 4 + 3 + 6 + 15 + 20 = 52
a + b + c + d + e + k + s; // Final expression statement
