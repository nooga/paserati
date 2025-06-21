// Uncaught exception from function call
function throwError() {
    throw "function threw error";
}
throwError();
// expect_runtime_error: function threw error