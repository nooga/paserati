// expect_compile_error: Cannot find module
import { missing } from './nonexistent';

const result = missing();
result;