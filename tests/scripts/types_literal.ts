// expect: true

type Method = "GET" | "POST";
type Status = 200 | 404 | 500;
type Truthy = true;

let reqMethod: Method = "GET";
// reqMethod = "PUT"; // Error: Type '"PUT"' is not assignable to type 'Method'.

let statusCode: Status = 200;
// statusCode = 400; // Error: Type '400' is not assignable to type 'Status'.

let isItTrue: Truthy = true;
// isItTrue = false; // Error: Type 'false' is not assignable to type 'Truthy'.

isItTrue;
