function test(): string {
  let result = "";
  
  try {
    result += "try ";
    return result + "try-return";
  } finally {
    result += "finally ";
    if (result.length > 10) {
      return result + "finally-return";
    }
  }
  
  return result + "normal-end";
}

console.log(test());