// expect: callback:5
// no-typecheck
// Bug: function declarations need to capture var declarations that appear LATER in source
// (because var is hoisted, the capture should work regardless of source order)
function Container() {
    // This function declaration appears BEFORE the var callback
    function callIt() {
        return "callback:" + callback();
    }

    // This var declaration appears AFTER the function that uses it
    var callback = function() { return 5; };

    this.run = function() {
        return callIt();
    }
}

var c = new Container();
c.run();
