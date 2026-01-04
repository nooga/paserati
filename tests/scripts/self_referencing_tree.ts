// Self-referencing interface - binary tree
// expect: 15

interface TreeNode {
  value: number;
  left: TreeNode | null;
  right: TreeNode | null;
}

// Create a simple tree:
//       5
//      / \
//     3   7
const leaf3: TreeNode = { value: 3, left: null, right: null };
const leaf7: TreeNode = { value: 7, left: null, right: null };
const root: TreeNode = { value: 5, left: leaf3, right: leaf7 };

// Sum all values - node is narrowed to TreeNode after null check
function sumTree(node: TreeNode | null): number {
  if (node === null) {
    return 0;
  }
  // node is now narrowed to TreeNode (not null)
  return node.value + sumTree(node.left) + sumTree(node.right);
}

sumTree(root);
