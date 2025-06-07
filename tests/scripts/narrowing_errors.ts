// expect_compile_error: no overlap

let x: "foo" | "bar" = "foo";

if (x === "foo") {
  if (x === "bar") {
    console.log("impossible");
  }
}

x;
