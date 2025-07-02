// Test case for default type parameter constraint bug
// When a default type parameter references another type parameter from the same declaration,
// the type checker should understand that if the referenced parameter satisfies its constraint,
// then it can be used as the default for another parameter with the same constraint

function add<DateType extends Date, ResultDate extends Date = DateType>(
  date: DateType,
  duration: { days?: number }
): ResultDate {
  // Simple implementation - just return the input date as result type
  return date as any;
}

// This should compile without constraint errors
function test() {
  const date = {} as Date;
  const result = add(date, { days: 10 });
  return result;
}

// expect: undefined
console.log("Constraint bug test passed");