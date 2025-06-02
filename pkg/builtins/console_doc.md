# Console Object Implementation

The console object in Paserati provides logging and debugging functionality similar to JavaScript/Node.js console.

## Implemented Methods

### Basic Logging

- `console.log(...args)` - Standard output logging
- `console.error(...args)` - Error logging with "ERROR: " prefix
- `console.warn(...args)` - Warning logging with "WARN: " prefix
- `console.info(...args)` - Info logging with "INFO: " prefix
- `console.debug(...args)` - Debug logging with "DEBUG: " prefix
- `console.trace(...args)` - Trace logging with "TRACE: " prefix

### Timing

- `console.time(label?)` - Start a timer (default label: "default")
- `console.timeEnd(label?)` - End a timer and display elapsed time in milliseconds

### Counting

- `console.count(label?)` - Increment and display a counter (default label: "default")
- `console.countReset(label?)` - Reset a counter to zero

### Grouping

- `console.group(...args)` - Start a collapsible group with optional label
- `console.groupCollapsed(...args)` - Start a collapsed group (same as group in our implementation)
- `console.groupEnd()` - End the current group

### Other

- `console.clear()` - Clear the console screen (ANSI escape sequence)

## Features

- **Smart String Formatting**: Top-level strings are unquoted, nested strings in objects/arrays are quoted
- **Complex Data Display**: Objects and arrays are formatted with proper nesting
- **Group Indentation**: Nested groups are properly indented with spaces
- **Label Support**: Timers and counters support custom labels
- **Type Safety**: All methods are properly typed in the type system

## Usage Examples

```typescript
// Basic logging
console.log("Hello", "world");
console.error("Something went wrong");

// Complex data
console.log("User:", { name: "John", scores: [95, 87] });

// Timing
console.time("operation");
// ... do work ...
console.timeEnd("operation"); // Outputs: operation: 1.234ms

// Counting
console.count("clicks"); // clicks: 1
console.count("clicks"); // clicks: 2

// Grouping
console.group("Main");
console.log("Item 1");
console.group("Sub");
console.log("Nested item");
console.groupEnd();
console.groupEnd();
```
