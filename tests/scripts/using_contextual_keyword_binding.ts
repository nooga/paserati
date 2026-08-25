// `using`/`await using` binding names accept the same contextual-keyword set
// TypeScript allows for any binding identifier (as, type, static, ...), not
// just IDENT/yield/get/throw/return. expectPeekIdentifierOrKeyword used to
// only recognize that narrow legacy set for every multi-declarator name
// (let/const/var/using), so `using as = ...` failed to parse at all.
let log: string[] = [];
function main() {
  using as = {
    [Symbol.dispose]() {
      log.push("dispose:as");
    },
  };
  using type = {
    [Symbol.dispose]() {
      log.push("dispose:type");
    },
  };
  log.push("body");
}
main();

log.join(",") === "body,dispose:type,dispose:as";
// expect: true
