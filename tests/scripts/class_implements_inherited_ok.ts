// expect: ok
interface N { name: string; greet?(): string }
interface M extends N { id: number }
class B { id = 1; }
class P extends B implements M { name = "x"; }
"ok";
