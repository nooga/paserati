// Regression: issue #239 -- a right-nested chain of binary `+` expressions
// (`x + (x + (x + ...))`, as opposed to the left-associative chains covered
// by issue #121 / long_operator_chain.ts) used to burn ~2 registers per
// nesting level, held live for the entire recursive descent into the right
// subtree, exhausting the 255-register allocator at roughly 130 levels deep
// and crashing the whole process with a raw Go panic. It now compiles in
// roughly half that (still O(depth) when every "pending left" operand needs
// its own register -- see right_nested_binary_purity_test.ts for why that
// floor is inherent, not a shortfall), comfortably covering this depth.
// expect: true

function inFunction(): number {
  let x = 1;
  let r = (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + x))))))))))))))))))))))))))))))))))))))))))))))))))))))))))))))))))))))))))))))))))))))))))))))))))))))))))))))))))))))))))))))))))))))))))))))))))))));
  return r;
}

let x = 1;
let r = (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + (x + x))))))))))))))))))))))))))))))))))))))))))))))))))))))))))))))))))))))))))))))))))))))))))))))))))))))))))))))))))))))))))))))))))))))))))))))))))))));
[r === 151, inFunction() === 151].every((v) => v);
