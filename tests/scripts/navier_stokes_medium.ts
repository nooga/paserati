// expect: done
// no-typecheck
// More complete reproduction of NavierStokes pattern with multiple nested functions

function FluidField() {
    function addFields(x, s, dt) {
        return x + s * dt;
    }

    function diffuse(b, x, x0, dt) {
        return x0 * dt;
    }

    function advect(b, d, d0, u, v, dt) {
        return d0;
    }

    function project(u, v, p, div) {
        return u + v;
    }

    function dens_step(x, x0, u, v, dt) {
        addFields(x, x0, dt);
        diffuse(0, x0, x, dt);
        addFields(x, x0, dt);
        advect(0, x, x0, u, v, dt);
    }

    function vel_step(u, v, u0, v0, dt) {
        addFields(u, u0, dt);
        addFields(v, v0, dt);
        diffuse(1, u0, u, dt);
        diffuse(2, v0, v, dt);
        project(u, v, u0, v0);
        advect(1, u, u0, u0, v0, dt);
        advect(2, v, v0, u0, v0, dt);
        project(u, v, u0, v0);
    }

    var uiCallback = function(d, u, v) {};

    function Field(dens, u, v) {
        this.dens = dens;
        this.u = u;
        this.v = v;
    }

    function queryUI(d, u, v) {
        uiCallback(new Field(d, u, v));
    }

    this.update = function() {
        queryUI(dens_prev, u_prev, v_prev);
        vel_step(u, v, u_prev, v_prev, dt);
        dens_step(dens, dens_prev, u, v, dt);
    };

    this.setUICallback = function(callback) {
        uiCallback = callback;
    };

    var iterations = 10;
    var visc = 0.5;
    var dt = 0.1;
    var dens = 0;
    var dens_prev = 0;
    var u = 0;
    var u_prev = 0;
    var v = 0;
    var v_prev = 0;
}

var solver = new FluidField();
solver.setUICallback(function(field) {});
solver.update();
"done";
