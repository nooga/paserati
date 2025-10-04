let obj = {valueOf: function() { console.log("valueOf called"); return 42; }}; console.log("obj.valueOf exists:", typeof obj.valueOf); let result = Number(obj); console.log("Result:", result);
