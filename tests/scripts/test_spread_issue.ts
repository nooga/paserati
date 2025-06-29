// Test object spread type inference issue

interface DataPoint {
    x: number;
    y: number;
}

const dataPoint: DataPoint = { x: 10, y: 20 };

// This should preserve all properties from dataPoint + add new ones
const enhanced = {
    ...dataPoint,
    color: "red",
    size: 5
};

// The type inference should show: { x: number; y: number; color: string; size: number }
// Access properties from both spread and new properties
enhanced.x + enhanced.color.length; // Should work without type errors

"Spread test completed";

// expect: Spread test completed