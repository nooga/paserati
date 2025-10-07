// var declarations are function-scoped (JavaScript semantics)
// At global level, var becomes a global variable
// expect: block

{
    var x = "block";
}
// var is function-scoped, so x is accessible at global level
x;