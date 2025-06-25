// Test object spread with function expressions
let getProps = () => ({x: 10, y: 20});
let nested = {inner: {z: 30}};
({...getProps(), ...nested.inner, w: 40});
// expect: {x: 10, y: 20, z: 30, w: 40}