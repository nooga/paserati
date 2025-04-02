// expect: 7

const fu = (x: number, y: boolean): number => {
  if (y) {
    return x + 1;
  } else {
    return x + 2;
  }
};

fu(2, true) + fu(2, false);
