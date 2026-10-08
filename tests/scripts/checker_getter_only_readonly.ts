// expect_compile_error: Cannot assign to 'area' because it is a read-only property
class Sq { constructor(private s: number) {} get area(): number { return this.s * this.s; } }
const q = new Sq(2);
q.area = 5;
