// Test Unicode whitespace handling
console.log("Testing Unicode characters:");

// Test vertical tab (0x000B)
console.log("Vertical tab char code:", "\u000B".charCodeAt(0));

// Test form feed (0x000C)
console.log("Form feed char code:", "\u000C".charCodeAt(0));

// Test line separator (0x2028)
console.log("Line separator char code:", "\u2028".charCodeAt(0));

// Test paragraph separator (0x2029)
console.log("Paragraph separator char code:", "\u2029".charCodeAt(0));

// Test eval with these characters
try {
  console.log("eval with vertical tab:", eval("-4\u000B>>\u000B1"));
} catch (e) {
  console.log("Error with vertical tab:", e.message);
}

try {
  console.log("eval with form feed:", eval("-4\u000C>>\u000C1"));
} catch (e) {
  console.log("Error with form feed:", e.message);
}

try {
  console.log("eval with line separator:", eval("-4\u2028>>\u20281"));
} catch (e) {
  console.log("Error with line separator:", e.message);
}

try {
  console.log("eval with paragraph separator:", eval("-4\u2029>>\u20291"));
} catch (e) {
  console.log("Error with paragraph separator:", e.message);
}
