// Debug Map methods test  
// expect: true

let map = new Map();
map.set("a", 1);
let deleteResult = map.delete("a");
deleteResult === true;