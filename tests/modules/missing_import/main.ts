// Test importing a module that doesn't exist
import { nonExistent } from "./this_module_does_not_exist";
console.log(nonExistent);

// expect_runtime_error: Failed to load module