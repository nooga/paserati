// Test for-of iteration over a union type that includes an array
// This handles the pattern: optional?.array after truthiness check

interface Item { id: string; }
interface Container {
  items?: Item[];
}

function process(c: Container): string {
  if (!c.items || c.items.length === 0) {
    return "empty";
  }
  const results: string[] = [];
  for (const item of c.items) {
    results.push(item.id);
  }
  return results.join(",");
}

process({ items: [{ id: "x" }, { id: "y" }] });

// expect: x,y
