// Type-Safe Expression Evaluator - AST + Pattern Matching + Recursion
// expect: ((2 + 3) * 4) = 20

// === Discriminated Union for Expression AST ===
interface NumExpr {
  kind: "num";
  value: number;
}

interface BinExpr {
  kind: "bin";
  op: string;
  left: Expr;
  right: Expr;
}

type Expr = NumExpr | BinExpr;

// === Smart Constructors with Type Inference ===
function num(value: number): NumExpr {
  return { kind: "num", value: value };
}

function add(left: Expr, right: Expr): BinExpr {
  return { kind: "bin", op: "+", left: left, right: right };
}

function mul(left: Expr, right: Expr): BinExpr {
  return { kind: "bin", op: "*", left: left, right: right };
}

// === Recursive Evaluator with Type Narrowing ===
function evaluate(expr: Expr): number {
  if (expr.kind === "num") {
    // Type narrowed to NumExpr
    return expr.value;
  }
  // Type narrowed to BinExpr (only remaining option)
  const left = evaluate(expr.left);
  const right = evaluate(expr.right);

  if (expr.op === "+") {
    return left + right;
  }
  if (expr.op === "*") {
    return left * right;
  }
  return 0;
}

// === Pretty Printer with Same Pattern ===
function print(expr: Expr): string {
  if (expr.kind === "num") {
    return "" + expr.value;
  }
  return "(" + print(expr.left) + " " + expr.op + " " + print(expr.right) + ")";
}

// === Build AST: (2 + 3) * 4 ===
const expr: Expr = mul(add(num(2), num(3)), num(4));

// === Evaluate and Print ===
print(expr) + " = " + evaluate(expr);
