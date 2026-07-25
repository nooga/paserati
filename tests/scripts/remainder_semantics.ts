// Remainder operator edge cases: the VM's integer fast path must preserve JS
// semantics exactly - truncated remainder with the dividend's sign, -0 for a
// negative dividend with zero remainder (observable via 1/x), NaN for a zero
// divisor, and fall-through to float math for non-integral operands.
// expect: 1,-1,1,-1,0,-Infinity,Infinity,1.5,NaN,NaN,2
let results: string[] = [];
results.push(String(7 % 3));
results.push(String(-7 % 3));
results.push(String(7 % -3));
results.push(String(-7 % -3));
results.push(String(0 % 5));
results.push(String(1 / (-7 % 7)));
results.push(String(1 / (7 % 7)));
results.push(String(5.5 % 2));
results.push(String(7 % 0));
results.push(String(Infinity % 2));
results.push(String(2 % Infinity));
results.join(",");
