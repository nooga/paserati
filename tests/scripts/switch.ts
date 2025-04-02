// expect: 35
// Test switch statements with fallthrough, break, default, and interaction with other features.

let a = 0;
let b = 0;
let x = 5; // Set x to trigger default case in the first switch

// First switch statement with break and default
switch (x) {
  case 1:
    a = 10;
    break;
  case 2:
    a = 20;
    break;
  case 3:
    a = 30;
    break;
  case 4:
    a = 50;
    break;
  default:
    a = 0; // x is 5, default case sets a to 0
}
// Variable 'a' should be 0 here.

// Loop containing the second switch statement
for (let i = 0; i < 5; i = i + 1) {
  switch (i) {
    case 0:
      b += 5; // i=0: b=5
      break;
    case 1:
      b += 10; // i=1: b=5+10=15. Fallthrough
    case 2:
      b += 15; // i=1: b=15+15=30; i=2: b=30+15=45
      break;
    case 3:
      b += 20; // i=3: b=45+20=65
      break;
    case 4:
      b -= 5; // i=4: b=65-5=60
      break;
    default:
      b = 999; // Should not happen
  }
}
// End of loop: Variable 'b' should be 60.

// Closure that uses switch
let getBonus = (val) => {
  let bonus = 0;
  switch (val) {
    case 60:
      bonus = 10; // b is 60, bonus is 10
      break;
    default:
      bonus = -10; // Should not happen
  }
  return bonus;
};

let extra = getBonus(b); // extra = 10

// Final result calculation
a + b + extra; // 0 + 45 - 10 = 35
