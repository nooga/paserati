// expect: date helper works
// Test .js extension resolving to .ts files (like date-fns does)
import { formatDate } from "./date-utils.js";
import { addDays } from "./time-utils.js";

// Both imports should resolve to .ts files even though .js extension is used
formatDate();