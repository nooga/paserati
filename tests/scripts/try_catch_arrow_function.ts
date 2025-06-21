// Try-catch inside arrow function
let testArrow = () => {
    try {
        throw new Error("arrow error");
    } catch (e) {
        return e.toString();
    }
};
testArrow();
// expect: Error: arrow error