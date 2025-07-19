// Test Performance API basic functionality
// expect: success

// Test performance.now()
let now1 = performance.now();

// Test performance.mark()
performance.mark("test-mark");

// Test performance.getEntriesByType()
let marks = performance.getEntriesByType("mark");

// Test performance.measure()
performance.measure("test-measure");

// Test performance.getEntriesByType() for measures
let measures = performance.getEntriesByType("measure");

// Test performance.clearMarks()
performance.clearMarks();

// Test performance.clearMeasures()
performance.clearMeasures();

"success";