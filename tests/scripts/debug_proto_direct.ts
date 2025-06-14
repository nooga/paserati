// expect: function

// Access Object.prototype directly
let ObjectProto = Object.getPrototypeOf({});
typeof ObjectProto.hasOwnProperty;