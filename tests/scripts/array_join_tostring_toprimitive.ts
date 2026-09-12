// expect: true
// ECMA-262 23.1.3.16 (Array.prototype.join) step 6 requires the full ToString
// abstract operation on a non-null/undefined element - which for an object
// means ToPrimitive("string") first (i.e. calling its own toString()/
// Symbol.toPrimitive), not just formatting the object's internal
// representation. joinElementToString previously called the low-level
// Value.ToString() directly, which produces "[object Object]" for any plain
// object regardless of a user-defined toString() override - it never invoked
// the override at all. Array.prototype.toString (23.1.3.36) defers to join,
// so it shared the same bug.
//
// Found chasing paserati#426: ajv@8.17.1's own `codegen` library (a widely
// used, real npm package) builds generated-code fragments as an array of
// `Name`/`_Code` wrapper objects (each with its own toString()) and joins
// them - under the old behavior this silently produced "[object Object]"
// fragments instead of the real generated identifiers, corrupting the
// output. Confirmed byte-for-byte identical to real Node for every case
// below, including toString-before-valueOf precedence and a throwing
// toString propagating as a real exception rather than being swallowed.
class WithToString {
  toString() {
    return "custom";
  }
}
class WithValueOf {
  valueOf() {
    return 42;
  }
}
class Throws {
  toString(): string {
    throw new Error("boom");
  }
}

const checks: boolean[] = [];

checks.push([new WithToString(), "x"].join("") === "customx");
// No toString override: ToPrimitive("string") tries toString first, which
// is the inherited Object.prototype.toString ("[object Object]") - it must
// NOT fall through to valueOf() just because one exists.
checks.push([new WithValueOf(), "x"].join("") === "[object Object]x");
checks.push(String([new WithToString()]) === "custom");
checks.push([new WithToString()].toString() === "custom");

let threw = false;
let message = "";
try {
  [new Throws()].join("");
} catch (e) {
  threw = true;
  message = (e as Error).message;
}
checks.push(threw && message === "boom");

checks.every((c) => c === true);
