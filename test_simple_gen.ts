const iter = {
  [Symbol.iterator]: function() {
    throw new Error("boom");
  }
};

try {
  const f = function*([x]) {};
  f(iter);
} catch (e) {
  console.log("caught:", e.message);
}
