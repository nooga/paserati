// expect: 765
// Practical example: Simple image pixel manipulation

// Create a small 2x2 RGBA image (4 bytes per pixel)
const imageData = new Uint8ClampedArray(16);

// Set pixel (0,0) to red
imageData[0] = 255;  // R
imageData[1] = 0;    // G  
imageData[2] = 0;    // B
imageData[3] = 255;  // A

// Set pixel (0,1) to green
imageData[4] = 0;    // R
imageData[5] = 255;  // G
imageData[6] = 0;    // B
imageData[7] = 255;  // A

// Set pixel (1,0) to blue
imageData[8] = 0;    // R
imageData[9] = 0;    // G
imageData[10] = 255; // B
imageData[11] = 255; // A

// Sum RGB values of first 3 pixels (ignore alpha)
imageData[0] + imageData[5] + imageData[10];