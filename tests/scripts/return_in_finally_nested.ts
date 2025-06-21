// Test 6: Nested finally returns
function test6() {
    try {
        try {
            return "inner try";
        } finally {
            return "inner finally";
        }
    } finally {
        return "outer finally";
    }
}

test6();
// expect: outer finally