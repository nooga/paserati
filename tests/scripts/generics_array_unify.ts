// Test that Array<T> and T[] are equivalent
let arr1: Array<string> = ["hello", "world"];
let arr2: string[] = arr1;  // Should work
let arr3: Array<string> = arr2;  // Should work

let nums1: Array<number> = [1, 2, 3];
let nums2: number[] = nums1;
let nums3: Array<number> = nums2;

// expect: null