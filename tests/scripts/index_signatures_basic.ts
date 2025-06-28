// expect: index signatures work

// Test basic index signature parsing and type checking

type StringMap = { [key: string]: string };
type NumberMap = { [index: number]: any };
type MixedType = { 
    name: string; 
    [key: string]: any;
};

"index signatures work";