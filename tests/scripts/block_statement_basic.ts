// Basic block statement test

let result = 0;
{
    // Block with simple assignment
    let temp = 42;
    result = temp;
}
result;
// expect: 42