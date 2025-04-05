// expect: [[8, 3, 13], 6, [[4, 7, 286], 55]]

// === Setup ===
let x = [10, 20, 30]; // Local Array
let y = 5; // Simple Local

// Function to test assignments on captured variables (upvalues)
function testUpvalues() {
  let z = [100, 200, 300]; // Captured Array
  let w = 50; // Captured Simple Variable

  function inner() {
    // --- Arithmetic ---
    z[0] += 1; // z[0] = 101
    z[1] -= 10; // z[1] = 190
    z[2] *= 2; // z[2] = 600
    z[0] /= 101; // z[0] = 1
    z[1] %= 90; // z[1] = 10 (190 % 90)
    z[2] **= 1; // z[2] = 600 (600^1)

    // --- Bitwise ---
    z[0] &= 3; // z[0] = 1 & 3 = 1
    z[1] |= 5; // z[1] = 10 | 5 = 15
    z[2] ^= 100; // z[2] = 600 ^ 100 = 564

    // --- Shift ---
    z[0] <<= 2; // z[0] = 1 << 2 = 4
    z[1] >>= 1; // z[1] = 15 >> 1 = 7
    z[2] >>>= 1; // z[2] = 564 >>> 1 = 282
    // Final z = [4, 7, 282]

    // --- Simple Upvalue ---
    w += 5; // w = 55
    w *= 1; // w = 55
    w -= 0; // w = 55
    w /= 1; // w = 55
    w %= 100; // w = 55
    w **= 1; // w = 55
    w &= 63; // w = 55 & 63 = 55
    w |= 0; // w = 55 | 0 = 55
    w ^= 0; // w = 55 ^ 0 = 55
    w <<= 0; // w = 55 << 0 = 55
    w >>= 0; // w = 55 >> 0 = 55
    w >>>= 0; // w = 55 >>> 0 = 55
    // Final w = 55
  }

  inner();
  return [z, w]; // Return modified captured vars
}

// === Apply Operations ===

// --- Simple Local 'y' ---
y += 3; // y = 8
y -= 1; // y = 7
y *= 2; // y = 14
y /= 7; // y = 2
y %= 1; // y = 0
y = 5; // Reset y
y **= 3; // y = 125
y &= 117; // y = 125 & 117 = 117
y |= 8; // y = 117 | 8 = 125
y ^= 100; // y = 125 ^ 100 = 25
y <<= 1; // y = 25 << 1 = 50
y >>= 2; // y = 50 >> 2 = 12
y >>>= 1; // y = 12 >>> 1 = 6
// Final y = 6

// --- Local Array 'x' ---
x[0] += 1; // x[0] = 11
x[1] -= 5; // x[1] = 15
x[2] *= 3; // x[2] = 90
x[0] /= 11; // x[0] = 1
x[1] %= 10; // x[1] = 5
x[2] **= 1; // x[2] = 90
x[0] &= 3; // x[0] = 1 & 3 = 1
x[1] |= 10; // x[1] = 5 | 10 = 15
x[2] ^= 64; // x[2] = 90 ^ 64 = 26
x[0] <<= 3; // x[0] = 1 << 3 = 8
x[1] >>= 2; // x[1] = 15 >> 2 = 3
x[2] >>>= 1; // x[2] = 26 >>> 1 = 13
// Final x = [8, 3, 13]

// --- Upvalues 'z', 'w' ---
let upvalueResult = testUpvalues();
// Final z = [4, 7, 282]
// Final w = 55

// === Final Result ===
// Combine results into a structure for the expectation check.
[x, y, upvalueResult]; // [[8, 3, 13], 6, [[4, 7, 286], 55]]
