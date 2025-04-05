// expect: 45456954545
function factorial(n) {
  return n === 1 ? n : n * factorial(--n);
}

let i = 0;
let output = 0;

while (i++ < 1e6) {
  output += factorial((i % 9) + 1);
}

output;
