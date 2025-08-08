try {
  JSON.parse("{invalid json}");
} catch(e) {
  console.log("Caught:", e.message);
}
console.log("Done");