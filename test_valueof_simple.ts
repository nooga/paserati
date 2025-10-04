let obj = {valueOf: function() { console.log("valueOf called"); throw "error"; }}; try { let result = Number(obj); console.log("Result:", result); } catch (e) { console.log("Caught:", e); }
