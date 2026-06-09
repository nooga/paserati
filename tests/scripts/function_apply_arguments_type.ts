// expect: 1
function checkedOnly() {
  function other() {}
  other.apply(this, arguments);
}

1;
