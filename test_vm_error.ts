// Test multiple error cases
function test() {
    let notAFunction: any = "hello";
    notAFunction(); // Should point to line 3
}
test();