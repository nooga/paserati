// Test importing from module
import defaultGreet, { message, greet } from './test_module';

console.log(message);
console.log(greet("World"));
console.log(defaultGreet());