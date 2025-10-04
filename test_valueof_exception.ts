try { let result = 1 + {valueOf: function() {throw "error"}, toString: function() {return 1}}; console.log("Should not reach here:", result); } catch (e) { console.log("Caught exception:", e); }
