// expect: called with field
// no-typecheck
// Minimal reproduction of NavierStokes pattern
function FluidField() {
    var uiCallback = function(d) {};

    function queryUI(d) {
        return uiCallback(d);
    }

    this.update = function() {
        return queryUI("field");
    };

    this.setUICallback = function(callback) {
        uiCallback = callback;
    };
}

var solver = new FluidField();
solver.setUICallback(function(field) {
    return "called with " + field;
});
solver.update();
