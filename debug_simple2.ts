try {
  throw new Error("test error");
} catch(e) {
  console.log("Caught:", e.message);
}