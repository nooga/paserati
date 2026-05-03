// expect: 25

namespace Geom {
  export class Point {
    x: number;
    y: number;
    constructor(x: number, y: number) {
      this.x = x;
      this.y = y;
    }
    distSq(): number {
      return this.x * this.x + this.y * this.y;
    }
  }
}

const p = new Geom.Point(3, 4);
p.distSq();
