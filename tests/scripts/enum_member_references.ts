// expect: 1,4,1,9,B|5|F|CAT-DOG
// Enum initializers may refer to earlier members (bare or as E.A), enums merge
// across declarations, and a computed member is evaluated at run time and gets
// its reverse mapping like any other numeric member.

enum E {
  A = 1,
  B = A << 2,
  C = "x".length,
  D = 9,
}

enum E {
  F = E.B | 1,
}

enum Names {
  Cat = "CAT",
  Dog = "DOG",
  Both = `${Cat}-DOG`,
}

const values = [E.A, E.B, E.C, E.D, E[4]];
values.join(",") + "|" + E.F + "|" + E[5] + "|" + Names.Both;
