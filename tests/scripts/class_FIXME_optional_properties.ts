// FIXME: Optional properties and parameters not yet supported
// expect: Alice (no email)
// Test optional properties and parameters

class User {
  name;
  email?; // FIXME: optional property
  age?; // FIXME: optional property

  constructor(name, email?, age?) {
    // FIXME: optional parameters
    this.name = name;
    this.email = email;
    this.age = age;
  }

  getInfo(includeAge?) {
    // FIXME: optional parameter
    let info = this.name;
    if (this.email) {
      info += ` (${this.email})`;
    } else {
      info += " (no email)";
    }
    if (includeAge && this.age) {
      info += `, age ${this.age}`;
    }
    return info;
  }
}

let user = new User("Alice");
user.getInfo();
