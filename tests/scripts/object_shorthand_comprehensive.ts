// expect: success

// Interface to test type compatibility
interface UserData {
  name: string;
  age: number;
  active: boolean;
}

// Test basic shorthand with type annotations
let name: string = "Alice";
let age: number = 25;
let active: boolean = true;

// Object using shorthand - should be compatible with interface
let user: UserData = { name, age, active };

// Test shorthand with functions
function createId(): number {
  return 42;
}

let id = createId();
let role = "admin";

// Mixed shorthand and regular properties
let profile = {
  id, // shorthand
  user, // shorthand (object)
  role, // shorthand
  permissions: ["read", "write"], // regular
  metadata: {
    // nested object with shorthand
    name, // shorthand from outer scope
    created: "2024-01-01",
  },
};

// Test property access and method calls
let greeting = "Hello, " + profile.user.name + "!";
let isAdmin = profile.role === "admin";

// Test with this context and method shorthand
let service = {
  name: "UserService",
  users: [user],

  // Method using shorthand properties in return
  getCurrentUser() {
    let currentUser = this.users[0];
    let name = currentUser.name;
    let age = currentUser.age;
    return { name, age, status: "online" };
  },
};

let current = service.getCurrentUser();

// Verify everything works
let result: string;
if (
  profile.id === 42 &&
  profile.user.name === "Alice" &&
  current.name === "Alice" &&
  isAdmin
) {
  result = "success";
} else {
  result = "failure";
}

result;
