// expect: TDZ|TDZ|TDZ|TDZ|TDZ|TDZ|TDZ|TDZ
// skip-typecheck
// The block, function-body and direct-eval halves of the non-first-declarator
// TDZ fix - see tdz_every_declarator.ts for the top-level half and the full
// explanation. `let a = 1, b = 2` used to mark only `a`, so reading `b` before
// the declaration reported "b is not defined" instead of a TDZ error.
//
// Type checking is skipped because the checker rejects a use-before-declaration
// inside a block or function outright ("Cannot find name 'b'", where tsc reports
// TS2448 and compiles it). That gap is pre-existing and identical for the first
// and non-first declarator, so it is not what this test is about - but it does
// mean these shapes cannot be expressed in a type-checked script.
//
// These paths give the unnamed message form ("Cannot access variable before
// initialization"), so the assertion is the error *kind*: a TDZ error rather
// than "not defined".

let out = [];

function classify(e) {
  const msg = String(e.message);
  if (msg.indexOf("before initialization") < 0) return "NOT-TDZ: " + msg;
  return "TDZ";
}

// --- function body ---
function fnBody(which) {
  try {
    if (which === 0) d;
    else e2;
  } catch (err) {
    return classify(err);
  }
  let d = 1,
    e2 = 2;
  return "NO-THROW";
}
out.push(fnBody(0));
out.push(fnBody(1));

// --- nested block ---
function inBlock(which) {
  {
    try {
      if (which === 0) f;
      else g;
    } catch (err) {
      return classify(err);
    }
    let f = 1,
      g = 2;
  }
  return "NO-THROW";
}
out.push(inBlock(0));
out.push(inBlock(1));

// --- const, through the same sites ---
function constInBlock(which) {
  {
    try {
      if (which === 0) h;
      else i;
    } catch (err) {
      return classify(err);
    }
    const h = 1,
      i = 2;
  }
  return "NO-THROW";
}
out.push(constInBlock(0));
out.push(constInBlock(1));

// --- direct eval, which hoists its own lexical bindings ---
function inEval(src) {
  try {
    eval(src);
  } catch (err) {
    return classify(err);
  }
  return "NO-THROW";
}
out.push(inEval("j; let j = 1, k = 2;"));
out.push(inEval("k; let j = 1, k = 2;"));

out.join("|");
