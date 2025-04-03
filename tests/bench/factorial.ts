function factorial(n) {
  return n === 1 ? n : n * factorial(--n);
}

let i = 0;

while (i++ < 1e6) {
  factorial(10);
}
