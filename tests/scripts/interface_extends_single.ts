// expect: {x: 10, y: 20, z: 30}

interface Point2D {
  x: number;
  y: number;
}

interface Point3D extends Point2D {
  z: number;
}

let point: Point3D = {
  x: 10,
  y: 20,
  z: 30,
};

point;
