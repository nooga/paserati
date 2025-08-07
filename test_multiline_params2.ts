// Test with active: boolean = true pattern
function createUser(
  name: string,
  age: number = 25,
  active: boolean = true
): string {
  return name;
}

createUser("test");