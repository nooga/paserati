// Test object spread with Proxy ownKeys trap
let callCount = 0;
const ownKeysResult = ['a', 'b', 'c'];

const proxy = new Proxy({a: 1, b: 2, c: 3, d: 4}, {
  ownKeys() {
    callCount++;
    return ownKeysResult;
  }
});

// Spread the proxy - should call ownKeys trap
const result = {...proxy};

// Verify ownKeys was called
console.log('ownKeys called:', callCount > 0);
console.log('result keys:', Object.keys(result));
console.log('result.a:', result.a);
console.log('result.d:', result.d); // d should NOT be included
