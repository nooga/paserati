// expect: 1417

let hex = 0xff; // 255
let binary = 0b101; // 5
let octal = 0o7; // 7
let largeDecimal = 1_000;
let scientific = 1.5e2; // 150

hex + binary + octal + largeDecimal + scientific;
