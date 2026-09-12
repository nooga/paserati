// expect: true
// paserati#426: compileClassDeclaration allocated a fresh `constructorReg`,
// stored it into the class's global slot or spill slot (matching how the
// class binding was predefined back in step 1), then returned without ever
// freeing it - regardless of whether the class had a superclass. A source
// file with many sequential class declarations (a common bundler/codegen
// output shape - e.g. @aws-sdk/client-s3's dist-cjs/index.js declares ~150
// `class XCommand extends ... {}` API command classes back to back inside
// its CJS wrapper function - a real, unmodified npm package) exhausted the
// 255-register budget at exactly 1 register held per class declaration,
// purely from this leak, with genuinely at most one such register ever
// live at once.
//
// Reproduced with a dependency-free synthetic repro matching that real
// shape: many distinct class declarations, each `extends` a base class via
// a fluent builder-style method chain (mirroring @aws-sdk/client-s3's own
// `class XCommand extends Base.classBuilder().ep({...}).build() {}`
// pattern), all inside a single wrapper function (CJS bundlers wrap each
// module's body in exactly this kind of function scope). Fixed by freeing
// constructorReg once its value is safely stored.
let body = "";
for (let i = 0; i < 300; i++) {
  body += `class Cmd${i} extends Base.classBuilder().ep({i:${i}}).build() {}\n`;
}
body =
  `class Base {
    static classBuilder() { return this; }
    static ep(x) { return this; }
    static build() { return class {}; }
  }
  ` + body + "return 300;";

const fn = new Function(body);
fn() === 300;
