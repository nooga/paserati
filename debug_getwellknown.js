console.log(typeof getWellKnownIntrinsicObject("%Array%"));
console.log(typeof getWellKnownIntrinsicObject("%Object%"));
try {
  console.log(
    typeof getWellKnownIntrinsicObject("%NotSoWellKnownIntrinsicObject%")
  );
} catch (e) {
  console.log("Error:", e);
}
