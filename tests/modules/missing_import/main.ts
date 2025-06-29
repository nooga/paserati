// expect_runtime_error: Failed to load module
import { missing } from './nonexistent';

const result = missing();
result;