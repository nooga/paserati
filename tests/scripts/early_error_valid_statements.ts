// expect: 11
// Valid code next to the statement-position, ASI and yield early errors: a let
// declaration split across lines, the do-while ASI special case, and a
// parenthesized yield used as an operand.
let
  n = 1;
do n += 1; while (false) n += 2
function* g(): Generator<number, void, number> {
  const v: number = (yield 1) + 1;
  n += v;
}
const it = g();
it.next();
it.next(4);
n += [1, 2].length;
if (n) { let inner = 1; n += inner - 1; }
n;
