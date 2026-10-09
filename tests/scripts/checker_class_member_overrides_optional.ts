// expect: true,true,true
interface Base {
  check?: () => boolean;
}
class Impl implements Base {
  check(): boolean {
    return true;
  }
}
class Sub extends Impl {}

abstract class AB {
  hook?(): boolean;
}
class Conc extends AB {
  hook(): boolean {
    return true;
  }
  run(): boolean {
    return this.hook();
  }
}
// an inherited optional member is still optional when not redeclared
class Plain extends AB {}
const p = new Plain();
const absent = p.hook ? p.hook() : true;
[new Sub().check(), new Conc().run(), absent].join(",");
