// expect: 6
// An interface member typed `typeof f`, where f is declared after the
// interface, is callable like f.
interface Utils {
  waitFor: typeof waitFor;
  double: typeof double;
}
function waitFor<T>(callback: () => T): T {
  return callback();
}
function double(n: number) {
  return n * 2;
}
const vi: Utils = { waitFor, double };
vi.waitFor(() => 2) + vi.double(2);
