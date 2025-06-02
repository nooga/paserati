// Simple rest parameter test with optional parameters
function formatMessage(
  message: string,
  urgent?: boolean,
  ...tags: string[]
): string {
  let result = message;
  if (urgent) {
    result = "[URGENT] " + result;
  }
  if (tags.length > 0) {
    result += " #" + tags.join(" #");
  }
  return result;
}

// This call was failing before our fix
console.log(
  'formatMessage("Test", true, "work", "important"):',
  formatMessage("Test", true, "work", "important")
);
console.log('formatMessage("Test"):', formatMessage("Test"));

// Also test simple rest parameters
function sum(...numbers: number[]): number {
  let total = 0;
  for (let i = 0; i < numbers.length; i++) {
    total += numbers[i];
  }
  return total;
}

console.log("sum(1, 2, 3):", sum(1, 2, 3));
