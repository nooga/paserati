// expect: function

let obj: any = {};
let proto = Object.getPrototypeOf(obj);
typeof proto.hasOwnProperty;