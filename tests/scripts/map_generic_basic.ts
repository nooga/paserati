// Test basic Map with generic types
// expect: success

// Test explicit generic types
let stringMap: Map<string, number> = new Map<string, number>();
stringMap.set("one", 1);
stringMap.set("two", 2);

// Type inference should work
let val1: number = stringMap.get("one") || 0;
let val2: number | undefined = stringMap.get("one");

// Test has method
let hasOne: boolean = stringMap.has("one");

// Test size property
let size: number = stringMap.size;

// Verify results
let result: string;
if (val1 === 1 && val2 === 1 && hasOne === true && size === 2) {
    result = "success";
} else {
    result = "failed";
}
result;