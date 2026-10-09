// expect: functionfunction2true
// A Proxy construct trap may return any object, including a function (#669)
const P = new Proxy(Object, { construct() { return function(){}; } });
const r = new P();
const F = new Proxy(Function, { construct(t, a) { return Reflect.construct(t, a); } });
const f = new F("a", "return a + 1");
let bad; try { new (new Proxy(Object,{construct(){return 1}}))(); } catch(e){ bad = e instanceof TypeError }
typeof r + typeof f + f(1) + bad;
