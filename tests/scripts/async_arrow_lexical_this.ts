// expect: obj
// An async arrow function must capture `this` lexically at creation time,
// exactly like a non-async arrow - never from the call site. Regression test
// for paserati#199, where an async arrow's `this` tracked the call's receiver
// (undefined for a bare call, the receiver object for a member call) as if
// the arrow flag were being ignored for async functions specifically.

const obj = {
    name: "obj",
    make() {
        return async () => this.name;
    },
};

const h = obj.make();
const holder = { h };

// Same function value, two different call shapes, neither receiver is `obj` -
// a correct lexical capture must return "obj" both times.
const bare = await h();
const member = await holder.h();

console.log("bare:", bare);
console.log("member:", member);

// The shape from the original report: an auto-bind class-field arrow handed
// off to a native as a detached callback (`agent.subscribe(this._handler)`).
// Promise.prototype.then calls it with no meaningful receiver of its own.
class Handler {
    name = "handler";
    onEvent = async () => this.name;
}
const detached = await Promise.resolve(undefined).then(new Handler().onEvent);
console.log("detached:", detached);

bare === "obj" && member === "obj" && detached === "handler"
    ? "obj"
    : "FAIL: bare=" + bare + " member=" + member + " detached=" + detached;
