// expect: hi hi
// A generic subclass, and its own generic subclass, can use a protected
// member of a generic base (#703).
abstract class A<D> {
    protected hello(): string { return "hi"; }
}
class B<D> extends A<D> {
    run(): string { return this.hello(); }
}
class C<T> extends B<T> {
    go(): string { return this.hello(); }
}
new B<number>().run() + " " + new C<string>().go();
