// Self-referencing interfaces (linked list pattern)
// expect: 3

interface ListNode {
  value: number;
  next: ListNode | null;
}

// Create a linked list
const node3: ListNode = { value: 3, next: null };
const node2: ListNode = { value: 2, next: node3 };
const node1: ListNode = { value: 1, next: node2 };

// Traverse to get last value using a loop
// (member expression narrowing like n.next is more complex)
function getLast(n: ListNode): number {
  let current: ListNode = n;
  while (current.next !== null) {
    current = current.next as ListNode;
  }
  return current.value;
}

getLast(node1);
