// expect_runtime_error: Uncaught exception: ReferenceError: uncaughtFromEvalXYZ is not defined
// no-typecheck
// A runtime exception thrown by indirect eval'd code with no surrounding
// try/catch must still terminate the script as a genuinely uncaught
// exception (not print as an internal engine error, and not silently
// succeed) - pins the message format vm.Interpret's InterpretRuntimeError
// handling produces once the exception re-crosses into the outer script and
// is never caught there either.
var indirectEval = eval;
indirectEval("uncaughtFromEvalXYZ;");
