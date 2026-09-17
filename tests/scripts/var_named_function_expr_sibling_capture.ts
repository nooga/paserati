// A named function expression assigned to `var` must remain visible to a
// sibling function defined afterward in the same scope, when the sibling
// closes over it as a free variable. The Annex-B predefine that lets a
// named function expression's body see the pre-hoisted `var` binding must
// not clobber a register the var-hoisting pass already assigned to it -
// doing so left the sibling's captured upvalue pointing at a different,
// still-undefined register. See paserati#472.
// expect: x

function factory() {
  var TokenType = function TokenType(label: string) {
    return label;
  };
  function binop(name: string) {
    return TokenType(name);
  }
  return binop("x");
}

factory();
