// Test that RegExp literal caching works correctly
// - Same pattern/flags should share compiled engine (performance)
// - But return distinct objects (spec compliance)

// expect: true

// Test in a loop - all should be different objects but all work
var regexes: RegExp[] = [];
for (var i = 0; i < 5; i++) {
  regexes.push(/test\d+/i);
}

// All objects should be distinct
var allDistinct = true;
for (var i = 0; i < regexes.length; i++) {
  for (var j = i + 1; j < regexes.length; j++) {
    if (regexes[i] === regexes[j]) {
      allDistinct = false;
    }
  }
}

// All should match correctly (shared compiled engine works)
var allWork = true;
allWork = allWork && regexes[0].test('TEST1');
allWork = allWork && regexes[1].test('test22');
allWork = allWork && regexes[2].test('TEST333');
allWork = allWork && regexes[3].test('test4444');
allWork = allWork && regexes[4].test('TEST55555');

// All should NOT match non-matching strings
var allReject = true;
allReject = allReject && !regexes[0].test('no match here');
allReject = allReject && !regexes[1].test('nothing');

// Different patterns should also work
var r1 = /abc/;
var r2 = /def/;
var differentPatterns = r1.test('abc') && r2.test('def') && !r1.test('def') && !r2.test('abc');

allDistinct && allWork && allReject && differentPatterns;
