// expect: true
// paserati#426 Symptom 1: nested if-statements previously held 1-2 live
// registers per nesting level for the entire duration of the nested
// compilation (a condition-temp register freed too late, plus a
// consequence/alternative scratch register that existed only to be copied
// into `hint` right after the nested compile finished, i.e. it was always
// going to hold exactly what `hint` should hold anyway). 150 levels of
// sequential nested `if`s - not an unusual amount of generated code (this is
// exactly the shape JSON-Schema-validator codegen and generated SDK clients
// produce) - exhausted the 255-register budget purely from nesting depth,
// with real engines handling this trivially. Fixed by freeing the condition
// register immediately after its one use (the conditional jump) and
// compiling each branch directly into `hint` instead of a scratch register.
//
// This test covers both the raw depth (well past the old ~118-level ceiling)
// and, since compiling directly into `hint` changes *how* completion values
// flow without being allowed to change *what* they are, a battery of the
// completion-value edge cases that distinguish "correct, just fewer
// registers" from "quietly wrong": break/continue copying a mid-branch
// value out of a loop, an if/else where only one branch produces a value,
// and a nested if whose inner branch is empty (hint must stay undefined,
// not leak the outer branch's own pre-init).
let body = "let x = 0;\n";
let close = "";
for (let i = 0; i < 1000; i++) {
  body += "if (data.a" + i + " !== undefined) { x += " + i + ";\n";
  close += "}\n";
}
body += close + "return x;";
const deepFn = new Function("data", body);

const depthOk = deepFn({}) === 0;

const breakVal = eval("for (let i=0;i<3;i++) { if (true) { 42; break; } }") === 42;
const continueVal = eval("for (let i=0;i<3;i++) { if (true) { 7; continue; } 9; }") === 7;
const elseOnlyVal = eval("if (false) { 1; } else { }") === undefined;
const thenOnlyVal = eval("if (true) { } else { 1; }") === undefined;
const nestedEmptyVal = eval("if (true) { if (true) { } }") === undefined;
const noElseFalseVal = eval("if (false) { 5; }") === undefined;
const ifElseTrueVal = eval("if (true) { 1; } else { 2; }") === 1;
const ifElseFalseVal = eval("if (false) { 1; } else { 2; }") === 2;

depthOk && breakVal && continueVal && elseOnlyVal && thenOnlyVal &&
  nestedEmptyVal && noElseFalseVal && ifElseTrueVal && ifElseFalseVal;
