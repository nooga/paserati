// expect: Animal: Dog named Buddy says Woof!
// Test class inheritance and method overriding

class Animal {
  name: string;
  species: string;

  constructor(name: string, species: string) {
    this.name = name;
    this.species = species;
  }

  speak(): string {
    return "Some generic animal sound";
  }

  info(): string {
    return `${this.species} named ${this.name}`;
  }
}

class Dog extends Animal {
  breed: string;

  constructor(name: string, breed: string) {
    super(name, "Dog");
    this.breed = breed;
  }

  // Override parent method
  speak(): string {
    return "Woof!";
  }

  // Call parent method
  describe(): string {
    return `Animal: ${super.info()} says ${this.speak()}`;
  }

  // New method specific to Dog
  wagTail(): string {
    return `${this.name} wags tail happily`;
  }
}

let dog = new Dog("Buddy", "Golden Retriever");
dog.describe();
