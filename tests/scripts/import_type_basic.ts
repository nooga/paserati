// Test basic import type functionality
// This tests that type-only imports work for simple interfaces

import type { Duration } from "../../../scratch/date-fns/src/types.js";

// Test 1: Basic type annotation
const duration: Duration = {
  days: 5,
  hours: 2
};

// Test 2: Type-only imports should not be available at runtime
// This would fail at runtime if Duration was imported as a value
// But it should compile fine with type-only import
function processDuration(d: Duration): number {
  return (d.days || 0) + (d.hours || 0);
}

const result = processDuration(duration);

// expect: undefined
console.log("Type-only import test passed");