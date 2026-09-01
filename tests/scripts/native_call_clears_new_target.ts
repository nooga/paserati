// expect: TypeError:true:TypeError:true:TypeError:true:string/number:object
// An ordinary [[Call]] carries no new.target - only [[Construct]] does - but
// vm.Call saved and set only `this`, so currentNewTarget and inConstructorCall
// leaked in from whatever native constructor was running above it on the Go
// stack. Every native callee reading them then behaved as though the *outer*
// constructor were constructing it (#154).
//
// The visible consequence: ToPrimitive's "neither toString nor valueOf is
// callable" step throws by constructing a TypeError through vm.Call, which
// resolved its prototype from new.target - still `String` here. The thrown
// object had name "TypeError" and the right message but String.prototype for
// its [[Prototype]], so `e instanceof TypeError` was false and
// e.constructor.name was "String".
const unconvertible: any = { valueOf: null, toString: null };

function classify(f: () => void): string {
  try {
    f();
    return "no-throw:false";
  } catch (e) {
    return e.name + ":" + (e instanceof TypeError);
  }
}

// The same leak reached natives called by *bytecode* running inside a native
// constructor, not just by the constructor's own Go code: the interpreter's
// plain-call path never touched these fields either, so the inner String("a")
// and Number("5") below saw inConstructorCall still true from the enclosing
// `new String` and returned wrapper objects instead of primitives.
const innerTypes = String(
  new String({
    toString() {
      return typeof String("a") + "/" + typeof Number("5");
    },
  } as any)
);

// ...while a genuine construct still constructs.
const realConstructStillWraps = typeof new String("z");

classify(() => new String(unconvertible)) + ":" +
  classify(() => new Number(unconvertible)) + ":" +
  classify(() => new Date(unconvertible)) + ":" +
  innerTypes + ":" + realConstructStillWraps;
