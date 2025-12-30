// expect: undefined
// This tests that if statements inside switch default cases parse correctly
function outer() {
  function inner() {
    const x = "b";
    switch (x) {
      default:
        if (true) {
          console.log("ok");
        }
    }
  }
  inner();
}
outer();
