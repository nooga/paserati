// expect: 1.5
function isPrivate(ip: string): boolean { return ip.startsWith("10."); }
let monthlyCost = isPrivate("10.0.0.1") ? 0.50 : 0.75;
monthlyCost += 1.00;
monthlyCost;
