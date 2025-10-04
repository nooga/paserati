try {
  true();
  console.log("ERROR: No exception thrown");
} catch (e) {
  console.log("Caught:", e);
  console.log("Type:", typeof e);
  console.log("Name:", e.name);
  console.log("Message:", e.message);
  console.log("instanceof TypeError:", e instanceof TypeError);
  console.log("instanceof Error:", e instanceof Error);
}
