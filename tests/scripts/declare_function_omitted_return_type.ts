// expect: 1
declare function value();

function checkedOnly() {
  let text: string = value();
  return text;
}

1;
