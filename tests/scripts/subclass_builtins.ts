// expect: 0
// Subclassing the built-in exotic kinds: a subclass instance carries its own
// [[Prototype]], and the read paths must consult it rather than the intrinsic.
// Several kinds had per-kind read blocks that walked straight to the intrinsic
// prototype, so none of the subclass's methods or getters were reachable.

let fails = 0;
function chk(name: string, cond: boolean): void {
  if (!cond) {
    fails++;
    console.log("FAIL", name);
  }
}

class MyRegExp extends RegExp {
  tag(): string {
    return "re";
  }
  get marker(): string {
    return "re-getter";
  }
}
const re: any = new MyRegExp("a");
chk("regexp method", re.tag() === "re");
chk("regexp getter", re.marker === "re-getter");

class MyBuffer extends ArrayBuffer {
  tag(): string {
    return "ab";
  }
}
const ab: any = new MyBuffer(8);
chk("arraybuffer method", ab.tag() === "ab");
chk("arraybuffer slice species", ab.slice(0, 4) instanceof MyBuffer);

class MyShared extends SharedArrayBuffer {
  tag(): string {
    return "sab";
  }
}
chk("sharedarraybuffer method", (new MyShared(8) as any).tag() === "sab");

class MyView extends DataView {
  tag(): string {
    return "dv";
  }
}
chk("dataview method", (new MyView(new ArrayBuffer(8)) as any).tag() === "dv");

class MyWeakMap extends WeakMap {
  tag(): string {
    return "wm";
  }
}
chk("weakmap method", (new MyWeakMap() as any).tag() === "wm");

class MyWeakSet extends WeakSet {
  tag(): string {
    return "ws";
  }
}
chk("weakset method", (new MyWeakSet() as any).tag() === "ws");

// Statics are inherited from a native parent constructor.
class SubPromise extends Promise {}
class SubBytes extends Uint8Array {}
class SubMap extends Map {}
chk("Promise statics", typeof (SubPromise as any).resolve === "function" && typeof (SubPromise as any).all === "function");
chk("TypedArray statics", typeof (SubBytes as any).from === "function" && typeof (SubBytes as any).of === "function");
chk("Map statics", typeof (SubMap as any).groupBy === "function");
chk("BYTES_PER_ELEMENT inherited", (SubBytes as any).BYTES_PER_ELEMENT === 1);

// An own property shadows the prototype's - [[Get]] checks own first. These
// kinds keep their own properties in a side table that the read path skipped.
const shadowed: any[] = [new Map(), new Set(), new Promise(() => {}), [], /a/];
for (const target of shadowed) {
  target.constructor = "OWN";
  chk("own constructor shadows prototype", target.constructor === "OWN");
}

// ...but an inherited getter-only accessor still wins over creating one:
// assigning through it is a silent no-op, not a new own data property.
const plainRe: any = /a/;
plainRe.global = "shifted";
chk("getter-only accessor rejects assignment", plainRe.global === false);
chk("no own property was created", Object.getOwnPropertyDescriptor(plainRe, "global") === undefined);

// Async function `constructor` is %AsyncFunction%, not %Function%. This is the
// regression guard for the hoist in handleCallableProperty: this VM gives async
// functions plain Function.prototype as their [[Prototype]] instead of
// %AsyncFunction.prototype%, so the static-inheritance chain walk would
// otherwise answer Function here.
const anAsyncFn: any = async function () {};
const AsyncFunction: any = anAsyncFn.constructor;
chk("async function constructor", AsyncFunction !== Function && AsyncFunction.name === "AsyncFunction");


fails;
