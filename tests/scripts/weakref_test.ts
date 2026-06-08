// WeakRef: construction with `new`, property lookup through the prototype
// chain, deref() type preservation across array / function / plain-object
// targets, and rejection of non-object targets / missing-new calls.

let arr = [10, 20, 30];
let wrArr = new WeakRef(arr);
let arrOk = wrArr.deref()!.length === 3 && wrArr.deref()![1] === 20 && Array.isArray(wrArr.deref());

let fn = () => 42;
let wrFn = new WeakRef(fn);
let fnOk = wrFn.deref()!() === 42;

let plain = { x: 7 };
let wrPlain = new WeakRef(plain);
let plainOk = wrPlain.deref()!.x === 7 && !Array.isArray(wrPlain.deref());

function rejects(thunk: () => unknown): boolean {
    try {
        thunk();
        return false;
    } catch (e) {
        return e instanceof TypeError;
    }
}

let rejectsNumber = rejects(() => new WeakRef(5 as any));
let rejectsNull = rejects(() => new WeakRef(null as any));
let rejectsMissingNew = rejects(() => (WeakRef as any)({}));

arrOk && fnOk && plainOk && rejectsNumber && rejectsNull && rejectsMissingNew;

// expect: true
