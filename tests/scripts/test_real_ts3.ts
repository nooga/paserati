function Person(this: { name: string }, name: string) {
    this.name = name;
}

// Test: Can we assign Person to a constructor type?
interface PersonConstructor {
    new(name: string): { name: string };
    prototype: any;
}

let ctor: PersonConstructor = Person; // Should work if Person is seen as constructor