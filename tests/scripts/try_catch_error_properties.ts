// Modifying Error properties in catch
let result;
try {
    let err = new Error("original message");
    err.code = "ERR001";
    throw err;
} catch (e) {
    e.name = "CustomError";
    result = e.toString();
}
result;
// expect: CustomError: original message