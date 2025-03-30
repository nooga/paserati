// expect: 8
const add = (x) => (y) => x + y;
const add5 = add(5);
add5(3);
