// `using`/`await using` bind a single disposable resource, so a
// destructuring pattern target is never valid. A for-of loop head commits to
// treating `using` as a declaration keyword unambiguously (there's nothing
// else it could mean there), so it's still recognized and reported even
// though the only declarator is the invalid pattern.

for (using {} of []) {
}

// expect_compile_error: 'using' declarations may not have binding patterns.
