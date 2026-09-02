// Subclassing Promise and setting an own instance property on `this` after
// super() must work the same way it already does for Array/Error/Map/Set.
// Regression test for paserati#198, where the VM's own-property-set switch
// had no case for TypePromise and fell through to a strict-mode TypeError.
// expect: ok
// no-typecheck

class APIPromise extends Promise {
    constructor(client, responsePromise, parseResponse) {
        super((resolve) => { resolve(null); });
        this.responsePromise = responsePromise;
        this.parseResponse = parseResponse;
    }
}

const p = new APIPromise({}, Promise.resolve({ value: 42 }), (c, d) => d.value);

const checks = [
    p.responsePromise instanceof Promise,
    typeof p.parseResponse === "function",
    p instanceof Promise,
    // The own property isn't just readable - it has to be callable through
    // OpCallMethod too (a different opcode from the plain-read path above).
    p.parseResponse("client", { value: 7 }) === 7,
];

const r = await p;

checks.push(r === null);

const resolved = await p.responsePromise;
checks.push(resolved.value === 42);

// A subclass method that shadows an intrinsic Promise.prototype method (e.g.
// `then`, the exact pattern @anthropic-ai/sdk's APIPromise uses) must resolve
// to the subclass's override, not the intrinsic - and `constructor` must
// report the subclass, not Promise.
class Overriding extends Promise {
    ranOverride = false;
    then(onFulfilled, onRejected) {
        this.ranOverride = true;
        return super.then(onFulfilled, onRejected);
    }
}
const o = new Overriding((resolve) => resolve(5));
checks.push(o.constructor === Overriding);
// Call .then() directly (rather than `await o`) so this only exercises
// property/method resolution on the subclass instance - `await`'s own
// PromiseResolve-thenable-job handling for a subclassed promise is a
// separate, wider spec-compliance question outside paserati#198's scope.
const overrideResult = await new Promise((resolve) => {
    o.then((v) => resolve(v));
});
checks.push(o.ranOverride === true);
checks.push(overrideResult === 5);

checks.every((c) => c === true) ? "ok" : "FAIL: " + JSON.stringify(checks);
