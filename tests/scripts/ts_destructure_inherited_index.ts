// expect: 3
// Destructuring must resolve inherited and index-signature members the same way
// member access does — neither should be reported as a missing property.

class Base { x: number = 1; }
class Derived extends Base { y: number = 2; }
const d = new Derived();
const { x, y } = d;

const bag: { [k: string]: number } = {};
const { foo } = bag;

x + y;
