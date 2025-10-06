// expect: true
// Test import.meta in module context

// import.meta should be an object
let meta = import.meta;
let result1 = typeof meta === "object";

// import.meta.url should be either undefined (non-module context) or a string (module context)
let urlType = typeof meta.url;
let result2 = urlType === "undefined" || urlType === "string";

// If url is defined, it should be non-empty
let result3 = meta.url === undefined || meta.url.length > 0;

// Return final result (not console.log which returns undefined)
result1 && result2 && result3;
