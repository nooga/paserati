// Simple nested template literal test
const result = `outer ${true ? `inner ${1 + 1}` : "fallback"} outer`;
console.log(result);