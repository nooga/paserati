// FIXME: Test continue to non-loop label should fail
// expect: undefined
// TODO: This should fail with compile error: continue statement cannot target non-loop label 'outer'

//outer: {
//    for (let i = 0; i < 3; i++) {
//        continue outer;  // Should fail - can't continue to block
//    }
//}
let result;
result;