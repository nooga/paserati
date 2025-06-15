// expect: 160

// Test keywords as property names in various contexts

// Object literal with keyword property names
let obj = {
  delete: 10,
  if: 20,
  function: 30,
  return: 40
};

// Object type with keyword property names  
type KeywordProps = {
  delete: number;
  if: number;
  function?: number;
};

// Interface with keyword property names
interface KeywordInterface {
  delete: number;
  if: number;
  while(x: number): number;
}

// Implementation
let impl: KeywordInterface = {
  delete: 50,
  if: 60,
  while(x: number): number {
    return x * 2;
  }
};

// Access keyword properties
let a = obj.delete;      // 10
let b = obj.if;          // 20  
let c = obj.function;    // 30
let d = obj.return;      // 40
let e = impl.delete;     // 50
let f = impl.while(5);   // 10

a + b + c + d + e + f;   // 10 + 20 + 30 + 40 + 50 + 10 = 160